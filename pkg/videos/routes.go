package videos

import (
	stream "github.com/b0nbon1/VidFlux/pkg/videos/services"
	"github.com/gofiber/fiber/v2"
)

var prefix = "/videos"

func VideosRoutes(router fiber.Router) {
	router.Get(prefix, GetVideos)
	router.Post(prefix, CreateVideo)
	// router.Get(prefix + "/stream", stream.StreamHandler)
	router.Post(prefix+"/process", stream.ProcessHLSHandler)
	router.Get(prefix+"/stream/:videoid", stream.ProcessFetchStream)
	router.Get(prefix+"/:videoid/segment/:segment", stream.FetchSegments)
	router.Get(prefix+"/:videoid/playlist/:playlist", stream.FetchPlaylist)

	// Resumable upload endpoints
	router.Post(prefix+"/upload/init", stream.InitUpload)
	router.Put(prefix+"/upload/:uploadId/chunk/:chunkIndex", stream.UploadChunk)
	router.Get(prefix+"/upload/:uploadId/status", stream.GetUploadStatus)
	router.Post(prefix+"/upload/:uploadId/complete", stream.CompleteUpload)
	router.Delete(prefix+"/upload/:uploadId", stream.CancelUpload)
	
}