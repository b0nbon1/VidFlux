package stream

import (
	// "bufio"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
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

	width, height, err := getVideoResolution(fmt.Sprintf("./%s/%s", folder, filename))
	if err != nil {
		log.Fatal("Error detecting video resolution:", err)
	}
	fmt.Printf("Detected resolution: %dx%d\n", width, height)

	cmd := generateFFmpegCommand(fmt.Sprintf("./%s/%s", folder, filename), folder, width, height)
	if err := cmd.Run(); err != nil {
		log.Fatal("Error running FFmpeg:", err)
	}

	fmt.Println("HLS video processing complete!")

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

func generateFFmpegCommand(inputFile string, outputDir string, width int, height int) *exec.Cmd {
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

			// Add map for video and audio
			maps = append(maps, fmt.Sprintf("[v%d]", streamIndex), "0:a")

			// Add video codec settings
			outputs = append(outputs,
				fmt.Sprintf("-c:v:%d", streamIndex), "libx264",
				fmt.Sprintf("-b:v:%d", streamIndex), res.bitrate,
				fmt.Sprintf("-maxrate:v:%d", streamIndex), res.maxrate,
				fmt.Sprintf("-bufsize:v:%d", streamIndex), res.bufsize,
				fmt.Sprintf("-c:a:%d", streamIndex), "aac",
				fmt.Sprintf("-b:a:%d", streamIndex), res.audioBr,
			)

			streamMap = append(streamMap, fmt.Sprintf("v:%d,a:%d,name:%dp", streamIndex, streamIndex, res.height))
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
		"-preset", "veryfast",
		"-sc_threshold", "0",
		"-g", "96",
		"-keyint_min", "96",
		"-force_key_frames", "expr:gte(t,n_forced*4)",
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

