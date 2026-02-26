package stream

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/matoous/go-nanoid/v2"
)

const hlsBasePath = "./static/videos"

func ProcessHLSHandler(c *fiber.Ctx) error {
	if form, err := c.MultipartForm(); err == nil {
		files := form.File["videos"]

		if len(files) == 0 {
			return c.SendStatus(fiber.StatusBadRequest)
		}

		file := files[0]

		fmt.Println(file.Filename, file.Size, file.Header["Content-Type"][0])

		folder, id := generateFolderWithId()
		filename := strings.ReplaceAll(file.Filename, " ", "-")

		if err := c.SaveFile(file, fmt.Sprintf("./%s/%s", folder, filename)); err != nil {
			return err
		}

		// Create encoding job for tracking
		GetTracker().CreateJob(id)

		// go hlsConversion(filename, folder, id)
		go hlsConversionWithResolutions(filename, folder, id)

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"message": "Success",
			"content": fiber.Map{
				"video_id": id,
				"file":     fmt.Sprintf("%s/%s", folder, filename),
			},
		})
	}

	return c.SendStatus(fiber.StatusInternalServerError)
}

func hlsConversionWithResolutions(filename string, folder string, id string) {
	tracker := GetTracker()
	inputPath := fmt.Sprintf("./%s/%s", folder, filename)

	// Get video duration first
	duration, err := getVideoDuration(inputPath)
	if err != nil {
		log.Printf("Error getting video duration: %v", err)
		tracker.MarkFailed(id, fmt.Sprintf("Error getting video duration: %v", err))
		return
	}
	tracker.SetTotalDuration(id, duration)

	width, height, err := getVideoResolution(inputPath)
	if err != nil {
		log.Printf("Error detecting video resolution: %v", err)
		tracker.MarkFailed(id, fmt.Sprintf("Error detecting video resolution: %v", err))
		return
	}
	fmt.Printf("Detected resolution: %dx%d\n", width, height)

	cmd := generateFFmpegCommand(inputPath, folder, width, height)

	// Capture stderr to parse progress
	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("Error creating stderr pipe: %v", err)
		tracker.MarkFailed(id, fmt.Sprintf("Error creating stderr pipe: %v", err))
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Error starting FFmpeg: %v", err)
		tracker.MarkFailed(id, fmt.Sprintf("Error starting FFmpeg: %v", err))
		return
	}

	// Parse FFmpeg progress output
	go parseFFmpegProgress(stderr, id, duration)

	if err := cmd.Wait(); err != nil {
		log.Printf("Error running FFmpeg: %v", err)
		tracker.MarkFailed(id, fmt.Sprintf("Error running FFmpeg: %v", err))
		return
	}

	tracker.MarkCompleted(id)
	fmt.Println("HLS video processing complete!")
}

func getVideoDuration(videoPath string) (float64, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_format", videoPath)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("error running ffprobe: %v", err)
	}

	var result struct {
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return 0, fmt.Errorf("error parsing ffprobe output: %v", err)
	}

	duration, err := strconv.ParseFloat(result.Format.Duration, 64)
	if err != nil {
		return 0, fmt.Errorf("error parsing duration: %v", err)
	}

	return duration, nil
}

func parseFFmpegProgress(stderr io.ReadCloser, videoID string, totalDuration float64) {
	tracker := GetTracker()
	scanner := bufio.NewScanner(stderr)

	// Regex patterns for FFmpeg output
	timeRegex := regexp.MustCompile(`time=(\d+):(\d+):(\d+\.?\d*)`)
	speedRegex := regexp.MustCompile(`speed=\s*(\d*\.?\d+)x`)

	for scanner.Scan() {
		line := scanner.Text()

		// Log all FFmpeg output for debugging
		log.Printf("[FFmpeg %s] %s", videoID, line)

		// Parse time
		if timeMatch := timeRegex.FindStringSubmatch(line); timeMatch != nil {
			hours, _ := strconv.ParseFloat(timeMatch[1], 64)
			minutes, _ := strconv.ParseFloat(timeMatch[2], 64)
			seconds, _ := strconv.ParseFloat(timeMatch[3], 64)
			currentTime := hours*3600 + minutes*60 + seconds

			// Parse speed
			speed := 1.0
			if speedMatch := speedRegex.FindStringSubmatch(line); speedMatch != nil {
				speed, _ = strconv.ParseFloat(speedMatch[1], 64)
			}

			tracker.UpdateProgress(videoID, currentTime, totalDuration, speed)
		}
	}
}


func HlsConversion(filename string, folder string, id string) {
	args := []string{"-i", filename, "-hls_time", "5", "-hls_playlist_type", "vod", "-hls_segment_filename", id + "%d.ts", "index.m3u8"}

	cmd := exec.Command("ffmpeg", args...)

	cmd.Dir = fmt.Sprintf("./%s", folder)

	out, err := cmd.Output()
	if err != nil {
		log.Fatal(err)
	} else {
		fmt.Printf("%s", out)
	}

	m3u8Path := filepath.Join(hlsBasePath, id, "index.m3u8")

	if _, err := os.Stat(m3u8Path); os.IsNotExist(err) {
		log.Fatal(err, "path not found!")
	}

	file, err := os.Open(m3u8Path)
	if err != nil {
		log.Fatal(err, "Error reading M3U8 file")
	}
	defer file.Close()

	var modifiedM3U8 string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if filepath.Ext(line) == ".ts" {
			line = fmt.Sprintf("/api/v1/videos/%s/segment/%s", id, line)
		}
		modifiedM3U8 += line + "\n"
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		log.Fatal(err, "Error scanning M3U8 file")
	}

	// Step 2: Overwrite the file with modified content
	err = os.WriteFile(m3u8Path, []byte(modifiedM3U8), 0644)
	if err != nil {
		log.Fatal(err, "Error writing to M3U8 file")
	}

	fmt.Println("File successfully updated!")
}

func generateFolderWithId() (string, string) {
	id, err := gonanoid.Generate("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", 6)
	if err != nil {
		return "", ""
	}

	if _, err := os.Stat(fmt.Sprintf("./static/videos/%s", id)); os.IsNotExist(err) {
		os.Mkdir(fmt.Sprintf("./static/videos/%s", id), 0755)
	} else {
		return generateFolderWithId()
	}

	return fmt.Sprintf("static/videos/%s", id), id
}

// GetEncodingStatus returns the current encoding progress for a video
func GetEncodingStatus(c *fiber.Ctx) error {
	videoID := c.Params("videoid")
	if videoID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "video_id is required",
		})
	}

	job := GetTracker().GetJob(videoID)
	if job == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "encoding job not found",
		})
	}

	response := fiber.Map{
		"video_id":       job.VideoID,
		"status":         job.Status,
		"progress":       job.Progress,
		"current_time":   job.CurrentTime,
		"total_duration": job.TotalDuration,
		"speed":          job.Speed,
		"eta_seconds":    job.ETA,
		"started_at":     job.StartedAt,
		"is_done":        job.Status == StatusCompleted || job.Status == StatusFailed,
	}

	if job.CompletedAt != nil {
		response["completed_at"] = job.CompletedAt
	}
	if job.Error != "" {
		response["error"] = job.Error
	}

	return c.JSON(response)
}

func ProcessFetchStream(c *fiber.Ctx) error {
	videoID := c.Params("videoid")
	m3u8Path := filepath.Join(hlsBasePath, videoID, "master.m3u8")
	content, err := os.ReadFile(m3u8Path)
	if err != nil {
		return c.Status(fiber.StatusNotFound).SendString("Stream not found")
	}

	c.Set("Content-Type", "application/vnd.apple.mpegurl")
	return c.SendString(string(content))
}

func FetchPlaylist(c *fiber.Ctx) error {
	videoID := c.Params("videoid")
	playlist := c.Params("playlist")
	playlistPath := filepath.Join(hlsBasePath, videoID, playlist)

	if _, err := os.Stat(playlistPath); os.IsNotExist(err) {
		return c.Status(fiber.StatusNotFound).SendString("Playlist not found")
	}

	c.Set("Content-Type", "application/vnd.apple.mpegurl")
	return c.SendFile(playlistPath)
}

func FetchSegments(c *fiber.Ctx) error {
	videoID := c.Params("videoid")
	segment := c.Params("segment")
	segmentPath := filepath.Join(hlsBasePath, videoID, segment)

	if _, err := os.Stat(segmentPath); os.IsNotExist(err) {
		return c.Status(fiber.StatusNotFound).SendString("Segment not found")
	}

	c.Set("Content-Type", "video/MP2T")
	return c.SendFile(segmentPath)
}


type VideoStream struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type ProbeResult struct {
	Streams []VideoStream `json:"streams"`
}

func getVideoResolution(videoPath string) (int, int, error) {
	cmd := exec.Command("ffprobe", "-v", "quiet", "-print_format", "json", "-show_streams", videoPath)
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return 0, 0, fmt.Errorf("error running ffprobe: %v", err)
	}

	var result ProbeResult
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		return 0, 0, fmt.Errorf("error parsing ffprobe output: %v", err)
	}

	// Find the highest resolution video stream
	var maxWidth, maxHeight int
	for _, stream := range result.Streams {
		if stream.Width > maxWidth {
			maxWidth = stream.Width
			maxHeight = stream.Height
		}
	}

	if maxWidth == 0 || maxHeight == 0 {
		return 0, 0, fmt.Errorf("could not determine video resolution")
	}

	return maxWidth, maxHeight, nil
}

func hasAudioStream(inputFile string) bool {
	cmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "a", "-show_entries", "stream=codec_type", "-of", "csv=p=0", inputFile)
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

func generateFFmpegCommand(inputFile string, outputDir string, width int, height int) *exec.Cmd {
	hasAudio := hasAudioStream(inputFile)
	log.Printf("Input file has audio: %v", hasAudio)

	resolutions := []struct {
		height  int
		width   int
		bitrate string
		maxrate string
		bufsize string
		audioBr string
	}{
		{360, 640, "800k", "856k", "1200k", "96k"},
		{480, 854, "1400k", "1498k", "2100k", "128k"},
		{720, 1280, "2800k", "2996k", "4200k", "128k"},
		{1080, 1920, "5000k", "5350k", "7500k", "192k"},
	}

	// Build filter_complex string and collect stream mappings
	var filters []string
	var maps []string
	var outputs []string
	var streamMap []string
	streamIndex := 0

	for _, res := range resolutions {
		if res.height <= height {
			// Add scale filter with padding to maintain aspect ratio
			filters = append(filters, fmt.Sprintf("[0:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2[v%d]",
				res.width, res.height, res.width, res.height, streamIndex))

			// Add map for video
			maps = append(maps, fmt.Sprintf("[v%d]", streamIndex))

			// Add map for audio only if it exists
			if hasAudio {
				maps = append(maps, "0:a")
			}

			// Add video codec settings
			outputs = append(outputs,
				fmt.Sprintf("-c:v:%d", streamIndex), "libx264",
				fmt.Sprintf("-b:v:%d", streamIndex), res.bitrate,
				fmt.Sprintf("-maxrate:v:%d", streamIndex), res.maxrate,
				fmt.Sprintf("-bufsize:v:%d", streamIndex), res.bufsize,
			)

			// Add audio codec settings only if audio exists
			if hasAudio {
				outputs = append(outputs,
					fmt.Sprintf("-c:a:%d", streamIndex), "aac",
					fmt.Sprintf("-b:a:%d", streamIndex), res.audioBr,
				)
				streamMap = append(streamMap, fmt.Sprintf("v:%d,a:%d,name:%dp", streamIndex, streamIndex, res.height))
			} else {
				streamMap = append(streamMap, fmt.Sprintf("v:%d,name:%dp", streamIndex, res.height))
			}
			streamIndex++
		}
	}

	// Build FFmpeg arguments
	args := []string{"-y", "-i", inputFile}

	// Add filter_complex
	args = append(args, "-filter_complex", strings.Join(filters, ";"))

	// Add all maps
	for _, m := range maps {
		args = append(args, "-map", m)
	}

	// Add all output settings
	args = append(args, outputs...)

	// Add encoding settings with fixed GOP for seamless quality switches
	// GOP = 4 seconds * 24fps = 96 frames (matching hls_time for segment alignment)
	args = append(args,
		"-preset", "slow",
		"-profile:v", "high",
		"-level", "4.1",
		"-crf", "18",
		"-sc_threshold", "0",
		"-g", "96",
		"-keyint_min", "96",
		"-force_key_frames", "expr:gte(t,n_forced*4)",
		"-pix_fmt", "yuv420p",
	)

	// Add HLS output settings (VOD mode - keep all segments, split on keyframes)
	args = append(args,
		"-f", "hls",
		"-hls_time", "4",
		"-hls_list_size", "0",
		"-hls_playlist_type", "vod",
		"-hls_flags", "independent_segments+split_by_time",
		"-hls_start_number_source", "epoch",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", fmt.Sprintf("%s/combined_%%v_%%03d.ts", outputDir),
		"-master_pl_name", "master.m3u8",
		"-var_stream_map", strings.Join(streamMap, " "),
		fmt.Sprintf("%s/combined_%%v.m3u8", outputDir),
	)

	// Print the full FFmpeg command (for debugging)
	fmt.Println("Running FFmpeg with arguments:", args)

	return exec.Command("ffmpeg", args...)
}

