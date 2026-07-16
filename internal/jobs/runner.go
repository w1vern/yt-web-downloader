package jobs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"yt-web-downloader/internal/ytapi"
	"yt-web-downloader/internal/ytdlp"
)

// runJob executes a job: it runs each yt-dlp pass, post-processes the
// resulting files (custom tags, final naming), writes the manifest, and
// sets the job's terminal state.
func (m *Manager) runJob(ctx context.Context, j *Job) {
	state, _, _, _ := j.Snapshot()
	if state != Queued {
		// job was deleted (or otherwise moved on) while queued
		return
	}

	jctx, cancel := context.WithCancel(ctx)
	j.mu.Lock()
	j.cancel = cancel
	j.mu.Unlock()
	defer cancel()

	j.setState(Running, "")

	if j.Opts.TagPlaylist && m.cfg.GoogleAPIKey != "" {
		j.publish(ytdlp.Event{Type: "log", Line: "Fetching playlist dates..."})
		dates, err := ytapi.FetchPlaylistDates(jctx, m.cfg.GoogleAPIKey, ytdlp.PlaylistID(j.Opts.URL), m.cfg.ProxyURL)
		if err != nil {
			j.publish(ytdlp.Event{Type: "log", Line: "WARN: playlist dates unavailable: " + err.Error()})
		} else {
			j.mu.Lock()
			j.PlaylistDates = dates
			j.mu.Unlock()
		}
	}

	cmds := ytdlp.BuildCommands(j.Opts, m.cfg.ProxyURL, m.cfg.CookiesFile)
	labels := ytdlp.PassLabels(j.Opts)

	for i, args := range cmds {
		label := labels[i]
		j.publish(ytdlp.Event{Type: "log", Line: "=== pass " + label + " ==="})
		if err := runPass(jctx, j, label, args); err != nil {
			j.setState(Failed, err.Error())
			_ = writeManifest(j.Dir, m.buildManifest(j, "error", err.Error()))
			cleanupTemp(j.Dir)
			return
		}
		postProcess(jctx, j, label)
	}

	cleanupTemp(j.Dir)

	files, err := scanFiles(j.Dir)
	if err != nil {
		j.setState(Failed, err.Error())
		_ = writeManifest(j.Dir, m.buildManifest(j, "error", err.Error()))
		return
	}
	j.mu.Lock()
	j.Files = files
	j.mu.Unlock()

	_ = writeManifest(j.Dir, m.buildManifest(j, "done", ""))
	j.setState(Done, "")
}

// buildManifest snapshots j into a Manifest with the given status/error.
func (m *Manager) buildManifest(j *Job, status, errMsg string) Manifest {
	_, _, finishedAt, files := j.Snapshot()
	return Manifest{
		ID:         j.ID,
		URL:        j.Opts.URL,
		Title:      j.Title,
		Mode:       j.Opts.Mode,
		Status:     status,
		Error:      errMsg,
		Files:      files,
		CreatedAt:  j.CreatedAt,
		FinishedAt: finishedAt,
	}
}

// runPass runs one yt-dlp invocation, streaming merged stdout+stderr lines
// as events. Exit code 1 counts as success if at least one unprocessed
// media file for this pass already exists in j.Dir (yt-dlp uses exit 1 for
// partial playlist failures under --ignore-errors); any other non-zero
// exit, or context cancellation, is an error.
func runPass(ctx context.Context, j *Job, pass string, args []string) error {
	cmd := exec.CommandContext(ctx, "yt-dlp", args...)
	cmd.Dir = j.Dir

	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pw.Close()
		return err
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- cmd.Wait()
		pw.Close()
	}()

	scanner := bufio.NewScanner(pr)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		j.publish(ytdlp.ParseLine(scanner.Text()))
	}

	if serr := scanner.Err(); serr != nil {
		j.publish(ytdlp.Event{Type: "log", Line: "WARN: reading yt-dlp output failed, aborting pass: " + serr.Error()})
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		// Drain whatever the child (or the internal stdout-copy goroutine)
		// is still trying to write, so it can observe EOF/kill and unblock;
		// otherwise cmd.Wait() below would never return.
		_, _ = io.Copy(io.Discard, pr)
		<-waitDone
		return fmt.Errorf("reading yt-dlp output: %w", serr)
	}

	waitErr := <-waitDone
	if waitErr == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) {
		if exitErr.ExitCode() == 1 && hasMediaFile(j.Dir, passExts[pass]) {
			j.publish(ytdlp.Event{Type: "log", Line: "WARN: pass exited 1 (partial failures), but media files were produced; continuing"})
			return nil
		}
		return fmt.Errorf("yt-dlp exited with code %d", exitErr.ExitCode())
	}
	return waitErr
}

// hasMediaFile reports whether dir contains at least one file that is a
// not-yet-post-processed output of the given pass: its extension must be
// in exts (the pass's whitelist) and its name must still carry the
// "[videoID]" segment that postProcess strips when it renames a file. This
// keeps the check pass-scoped, so an earlier pass's (already renamed)
// output can't mask a later pass that produced nothing.
func hasMediaFile(dir string, exts map[string]bool) bool {
	if len(exts) == 0 {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !exts[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		if !idFileRe.MatchString(name) {
			continue
		}
		return true
	}
	return false
}

var idFileRe = regexp.MustCompile(`\[([A-Za-z0-9_-]{11})\]\.[^.]+$`)

var passExts = map[string]map[string]bool{
	"audio": {".opus": true, ".ogg": true, ".mp3": true, ".m4a": true, ".webm": true},
	"video": {".mp4": true, ".mkv": true, ".webm": true},
	"av":    {".mp4": true, ".mkv": true, ".webm": true},
}

// postProcess tags and renames the files produced by one pass. Static
// per-video tags (URL, DATE, PLAYLIST_URL) are written by yt-dlp itself via
// --parse-metadata (see internal/ytdlp/command.go); the only tag applied
// here is PLAYLIST_DATE, since playlist upload dates are only known after
// the ytapi lookup that runs before yt-dlp starts.
func postProcess(ctx context.Context, j *Job, pass string) {
	exts := passExts[pass]
	if exts == nil {
		exts = map[string]bool{}
	}

	entries, err := os.ReadDir(j.Dir)
	if err != nil {
		j.publish(ytdlp.Event{Type: "log", Line: "WARN: post-process: " + err.Error()})
		return
	}

	j.mu.Lock()
	playlistDates := j.PlaylistDates
	j.mu.Unlock()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		m := idFileRe.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		if !exts[strings.ToLower(filepath.Ext(name))] {
			continue
		}
		id := m[1]
		src := filepath.Join(j.Dir, name)

		tags := map[string]string{}
		if j.Opts.TagPlaylist && len(playlistDates) > 0 {
			if d, ok := playlistDates[id]; ok && d != "" {
				tags["PLAYLIST_DATE"] = d
			}
		}

		suffix := ytdlp.TypeSuffix(pass, strings.ToLower(filepath.Ext(name)), j.Opts)
		dst := finalName(name, suffix)
		dstPath := filepath.Join(j.Dir, dst)
		if _, statErr := os.Stat(dstPath); statErr == nil {
			// collision: keep the " [id]" segment in the destination name
			dst = finalNameKeepID(name, suffix)
			dstPath = filepath.Join(j.Dir, dst)
		}

		if len(tags) > 0 {
			if err := tagWithFFmpeg(ctx, src, dstPath, tags); err != nil {
				j.publish(ytdlp.Event{Type: "log", Line: "WARN: ffmpeg tagging failed for " + name + ": " + err.Error()})
				if err := plainRename(src, dstPath); err != nil {
					j.publish(ytdlp.Event{Type: "log", Line: "WARN: rename failed for " + name + ": " + err.Error()})
					continue
				}
			}
		} else {
			if err := plainRename(src, dstPath); err != nil {
				j.publish(ytdlp.Event{Type: "log", Line: "WARN: rename failed for " + name + ": " + err.Error()})
				continue
			}
		}

		j.publish(ytdlp.Event{Type: "log", Line: "file ready: " + filepath.Base(dstPath)})
	}
}

// plainRename renames src to dst, mapping a .opus source to a .ogg
// destination extension (same container, safe rename) if dst doesn't
// already carry the right extension.
func plainRename(src, dst string) error {
	return os.Rename(src, dst)
}

// tagWithFFmpeg remuxes src into dstPath's extension via ffmpeg, embedding
// the given metadata tags, then removes src. It uses the job context so an
// in-flight remux is killed if the job is cancelled/deleted.
//
// .ogg (opus) destinations get a dedicated path: yt-dlp/mutagen stores the
// embedded cover as a vorbis-comment METADATA_BLOCK_PICTURE, which ffmpeg
// demuxes as an attached-pic PNG stream that ITS OWN ogg muxer cannot write
// back out ("Unsupported codec id in stream 1"). So for .ogg we (a) extract
// the cover to a standalone PNG if one exists, (b) dump the source's
// existing tags to an ffmetadata file and append the new tags plus the
// cover (as a METADATA_BLOCK_PICTURE line, built in Go — see flacpic.go) to
// it, and (c) remux audio-only with -map 0:a against that ffmetadata file
// via -map_metadata, instead of asking ffmpeg to mux a video stream into
// ogg or passing the ~1.6MB base64 cover as a single "-metadata" argv
// argument (which exceeds the per-arg length limit).
func tagWithFFmpeg(ctx context.Context, src, dstPath string, tags map[string]string) error {
	ext := filepath.Ext(dstPath)
	base := strings.TrimSuffix(dstPath, ext)
	if strings.EqualFold(ext, ".ogg") {
		return tagOggWithCover(ctx, src, base, dstPath, tags)
	}

	// Keep the real extension as the final suffix so ffmpeg can still infer
	// the right muxer from the output filename; the ".tmp." in the middle
	// is what cleanupTemp's "*.tmp.*" pattern sweeps up if the process dies
	// mid-remux and leaves this file behind.
	tmp := base + ".tmp" + ext
	args := []string{"-y", "-nostdin", "-i", src, "-map", "0", "-c", "copy"}
	args = append(args, metadataArgs(tags)...)
	args = append(args, tmp)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: %s", err, truncate(string(out), 300))
	}
	if err := os.Rename(tmp, dstPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Remove(src); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// tagOggWithCover implements the .ogg-specific path described on
// tagWithFFmpeg: extract the cover (if any) to a PNG, dump the existing
// tags to an ffmetadata file, append the new tags (plus the cover, as a
// METADATA_BLOCK_PICTURE line) to that file, and remux using
// -map_metadata against the file. The cover can be ~1.6MB base64'd, which
// is well past the per-argv-arg limit some platforms enforce on ffmpeg's
// "-metadata k=v" form, so the ffmetadata-file route is used instead of
// passing it directly on the command line.
func tagOggWithCover(ctx context.Context, src, base, dstPath string, tags map[string]string) error {
	ext := filepath.Ext(dstPath)
	coverTmp := base + ".cover.tmp.png"
	defer os.Remove(coverTmp)

	var picture string
	extractArgs := []string{"-y", "-nostdin", "-i", src, "-map", "0:v", "-frames:v", "1", "-c", "copy", coverTmp}
	if _, err := exec.CommandContext(ctx, "ffmpeg", extractArgs...).CombinedOutput(); err == nil {
		if png, rerr := os.ReadFile(coverTmp); rerr == nil {
			if b64, perr := flacPictureBlock(png); perr == nil {
				picture = b64
			}
		}
	}
	// A failed/missing extraction just means no cover to preserve; the
	// remux below still proceeds.

	// metaTmp ends in ".tmp" so cleanupTemp's "*.tmp" pattern sweeps up any
	// leftover if the process dies mid-remux.
	metaTmp := base + ".meta.tmp"
	defer os.Remove(metaTmp)

	dumpArgs := []string{"-y", "-nostdin", "-i", src, "-f", "ffmetadata", "-map_metadata:g", "0:s:0", metaTmp}
	if out, err := exec.CommandContext(ctx, "ffmpeg", dumpArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, truncate(string(out), 300))
	}

	if err := appendFFMetadata(metaTmp, tags, picture); err != nil {
		return err
	}

	tmp := base + ".tmp" + ext
	args := []string{"-y", "-nostdin", "-i", src, "-f", "ffmetadata", "-i", metaTmp,
		"-map_metadata", "1", "-map", "0:a", "-c", "copy", tmp}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("%w: %s", err, truncate(string(out), 300))
	}
	if err := os.Rename(tmp, dstPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Remove(src); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// appendFFMetadata appends tags (sorted for deterministic output) and, if
// picture is non-empty, a METADATA_BLOCK_PICTURE line, to the ffmetadata
// file at path. Keys are written as-is; values are escaped per ffmpeg's
// ffmetadata syntax.
func appendFFMetadata(path string, tags map[string]string, picture string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, k := range sortedKeys(tags) {
		if _, err := fmt.Fprintf(f, "%s=%s\n", k, escapeFFMetadataValue(tags[k])); err != nil {
			return err
		}
	}
	if picture != "" {
		if _, err := fmt.Fprintf(f, "METADATA_BLOCK_PICTURE=%s\n", escapeFFMetadataValue(picture)); err != nil {
			return err
		}
	}
	return nil
}

// escapeFFMetadataValue backslash-escapes the characters ffmpeg's
// ffmetadata format treats specially in values: '=', ';', '#', '\\', and
// newline.
func escapeFFMetadataValue(v string) string {
	var b strings.Builder
	for _, r := range v {
		switch r {
		case '=', ';', '#', '\\', '\n':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sortedKeys returns m's keys in sorted order (deterministic arg/output
// order, which helps tests/debugging).
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// metadataArgs turns tags into sorted "-metadata k=v" ffmpeg args
// (sorted for deterministic arg order, which helps tests/debugging).
func metadataArgs(tags map[string]string) []string {
	var args []string
	for _, k := range sortedKeys(tags) {
		args = append(args, "-metadata", k+"="+tags[k])
	}
	return args
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// cleanupTemp removes yt-dlp's transient/sidecar files from dir, plus this
// package's own temp files. The remux/tag temp names are listed precisely
// (rather than via an infix glob like "*.tmp.*") so a final output file
// whose title happens to contain the substring ".tmp." is never swept up:
//   - tagWithFFmpeg / tagOggWithCover's remux temp: base+".tmp"+ext, where
//     ext is one of the pass extensions in passExts (.opus destinations are
//     always renamed to .ogg before reaching here, so ".tmp.opus" never
//     occurs)
//   - tagOggWithCover's extracted-cover temp: base+".cover.tmp.png"
//   - tagOggWithCover's ffmetadata dump temp: base+".meta.tmp" (also
//     removed directly via defer, but covered here too in case the process
//     dies mid-remux)
func cleanupTemp(dir string) {
	patterns := []string{
		"*.part", "*.tmp", "*.ytdl", "*.webp", "*.jpg", "*.png", "*.json",
		"*.cover.tmp.png", "*.meta.tmp",
		"*.tmp.ogg", "*.tmp.mp3", "*.tmp.m4a",
		"*.tmp.mp4", "*.tmp.mkv", "*.tmp.webm",
	}
	for _, pat := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			continue
		}
		for _, mpath := range matches {
			_ = os.Remove(mpath)
		}
	}
}

// scanFiles lists the (non-directory) contents of dir as sorted FileInfo.
func scanFiles(dir string) ([]FileInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []FileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == manifestFileName || strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, FileInfo{Name: e.Name(), Size: info.Size()})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

var idSuffixRe = regexp.MustCompile(` \[[A-Za-z0-9_-]{11}\]$`)

// finalName turns "Chan - Title [abcdefghijk].opus" + "audio opus"
// into "Chan - Title - audio opus.ogg".
func finalName(orig, suffix string) string {
	ext := filepath.Ext(orig)
	base := strings.TrimSuffix(orig, ext)
	base = idSuffixRe.ReplaceAllString(base, "")
	if ext == ".opus" {
		ext = ".ogg"
	}
	return base + " - " + suffix + ext
}

// finalNameKeepID is like finalName but preserves the " [id]" segment,
// used to avoid collisions when two source files would otherwise map to
// the same destination name.
func finalNameKeepID(orig, suffix string) string {
	ext := filepath.Ext(orig)
	base := strings.TrimSuffix(orig, ext)
	if ext == ".opus" {
		ext = ".ogg"
	}
	return base + " - " + suffix + ext
}
