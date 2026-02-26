package stream

import (
	"sync"
	"time"
)

type EncodingStatus string

const (
	StatusPending    EncodingStatus = "pending"
	StatusProcessing EncodingStatus = "processing"
	StatusCompleted  EncodingStatus = "completed"
	StatusFailed     EncodingStatus = "failed"
)

type EncodingJob struct {
	VideoID       string         `json:"video_id"`
	Status        EncodingStatus `json:"status"`
	Progress      float64        `json:"progress"`       // 0-100
	CurrentTime   float64        `json:"current_time"`   // seconds encoded
	TotalDuration float64        `json:"total_duration"` // total video duration in seconds
	Speed         float64        `json:"speed"`          // encoding speed (e.g., 2.5x)
	ETA           int64          `json:"eta"`            // estimated seconds remaining
	StartedAt     time.Time      `json:"started_at"`
	CompletedAt   *time.Time     `json:"completed_at,omitempty"`
	Error         string         `json:"error,omitempty"`
}

type EncodingTracker struct {
	jobs map[string]*EncodingJob
	mu   sync.RWMutex
}

var tracker = &EncodingTracker{
	jobs: make(map[string]*EncodingJob),
}

func GetTracker() *EncodingTracker {
	return tracker
}

func (t *EncodingTracker) CreateJob(videoID string) *EncodingJob {
	t.mu.Lock()
	defer t.mu.Unlock()

	job := &EncodingJob{
		VideoID:   videoID,
		Status:    StatusPending,
		Progress:  0,
		StartedAt: time.Now(),
	}
	t.jobs[videoID] = job
	return job
}

func (t *EncodingTracker) GetJob(videoID string) *EncodingJob {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if job, exists := t.jobs[videoID]; exists {
		return job
	}
	return nil
}

func (t *EncodingTracker) UpdateProgress(videoID string, currentTime, totalDuration, speed float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if job, exists := t.jobs[videoID]; exists {
		job.Status = StatusProcessing
		job.CurrentTime = currentTime
		job.TotalDuration = totalDuration
		job.Speed = speed

		if totalDuration > 0 {
			job.Progress = (currentTime / totalDuration) * 100
			if job.Progress > 100 {
				job.Progress = 100
			}
		}

		// Calculate ETA
		if speed > 0 && totalDuration > 0 {
			remainingTime := totalDuration - currentTime
			job.ETA = int64(remainingTime / speed)
		}
	}
}

func (t *EncodingTracker) SetTotalDuration(videoID string, duration float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if job, exists := t.jobs[videoID]; exists {
		job.TotalDuration = duration
	}
}

func (t *EncodingTracker) MarkCompleted(videoID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if job, exists := t.jobs[videoID]; exists {
		job.Status = StatusCompleted
		job.Progress = 100
		now := time.Now()
		job.CompletedAt = &now
		job.ETA = 0
	}
}

func (t *EncodingTracker) MarkFailed(videoID string, err string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if job, exists := t.jobs[videoID]; exists {
		job.Status = StatusFailed
		job.Error = err
		now := time.Now()
		job.CompletedAt = &now
	}
}

func (t *EncodingTracker) DeleteJob(videoID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.jobs, videoID)
}
