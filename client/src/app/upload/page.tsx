"use client";

import { useState, useCallback, useRef, useEffect } from "react";
import styles from "./upload.module.css";

const API_BASE = "http://127.0.0.1:4500/api/v1/videos";
const CHUNK_SIZE = 5 * 1024 * 1024; // 5MB chunks

interface UploadSession {
  uploadId: string;
  totalChunks: number;
  chunkSize: number;
}

interface UploadState {
  file: File | null;
  status: "idle" | "uploading" | "paused" | "processing" | "encoding" | "complete" | "error";
  progress: number;
  uploadedChunks: number;
  totalChunks: number;
  videoId: string | null;
  error: string | null;
  speed: string;
}

interface EncodingStatus {
  video_id: string;
  status: "pending" | "processing" | "completed" | "failed";
  progress: number;
  current_time: number;
  total_duration: number;
  speed: number;
  eta_seconds: number;
  error?: string;
}

export default function UploadPage() {
  const [uploadState, setUploadState] = useState<UploadState>({
    file: null,
    status: "idle",
    progress: 0,
    uploadedChunks: 0,
    totalChunks: 0,
    videoId: null,
    error: null,
    speed: "",
  });

  const [isDragging, setIsDragging] = useState(false);
  const [encodingProgress, setEncodingProgress] = useState<EncodingStatus | null>(null);
  const sessionRef = useRef<UploadSession | null>(null);
  const abortControllerRef = useRef<AbortController | null>(null);
  const isPausedRef = useRef(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const encodingPollRef = useRef<NodeJS.Timeout | null>(null);

  // Cleanup polling on unmount
  useEffect(() => {
    return () => {
      if (encodingPollRef.current) {
        clearInterval(encodingPollRef.current);
      }
    };
  }, []);

  const pollEncodingStatus = useCallback((videoId: string) => {
    // Clear any existing polling
    if (encodingPollRef.current) {
      clearInterval(encodingPollRef.current);
    }

    const poll = async () => {
      try {
        const response = await fetch(`${API_BASE}/encoding/${videoId}/status`);
        if (!response.ok) {
          return;
        }

        const data: EncodingStatus = await response.json();
        setEncodingProgress(data);

        if (data.status === "completed") {
          if (encodingPollRef.current) {
            clearInterval(encodingPollRef.current);
            encodingPollRef.current = null;
          }
          setUploadState((prev) => ({
            ...prev,
            status: "complete",
          }));
        } else if (data.status === "failed") {
          if (encodingPollRef.current) {
            clearInterval(encodingPollRef.current);
            encodingPollRef.current = null;
          }
          setUploadState((prev) => ({
            ...prev,
            status: "error",
            error: data.error || "Encoding failed",
          }));
        }
      } catch (error) {
        console.error("Error polling encoding status:", error);
      }
    };

    // Poll immediately, then every 2 seconds
    poll();
    encodingPollRef.current = setInterval(poll, 2000);
  }, []);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(true);
  }, []);

  const handleDragLeave = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);
  }, []);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    setIsDragging(false);

    const files = e.dataTransfer.files;
    if (files.length > 0) {
      const file = files[0];
      if (file.type.startsWith("video/")) {
        selectFile(file);
      } else {
        setUploadState((prev) => ({
          ...prev,
          error: "Please select a video file",
        }));
      }
    }
  }, []);

  const handleFileSelect = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (files && files.length > 0) {
        selectFile(files[0]);
      }
    },
    []
  );

  const selectFile = (file: File) => {
    setUploadState({
      file,
      status: "idle",
      progress: 0,
      uploadedChunks: 0,
      totalChunks: Math.ceil(file.size / CHUNK_SIZE),
      videoId: null,
      error: null,
      speed: "",
    });
    sessionRef.current = null;
  };

  const initUpload = async (file: File): Promise<UploadSession | null> => {
    try {
      const response = await fetch(`${API_BASE}/upload/init`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          filename: file.name,
          total_size: file.size,
          chunk_size: CHUNK_SIZE,
        }),
      });

      if (!response.ok) {
        throw new Error("Failed to initialize upload");
      }

      const data = await response.json();
      return {
        uploadId: data.upload_id,
        totalChunks: data.total_chunks,
        chunkSize: data.chunk_size,
      };
    } catch (error) {
      console.error("Init upload error:", error);
      return null;
    }
  };

  const uploadChunk = async (
    uploadId: string,
    chunkIndex: number,
    chunk: Blob,
    signal: AbortSignal
  ): Promise<boolean> => {
    try {
      const response = await fetch(
        `${API_BASE}/upload/${uploadId}/chunk/${chunkIndex}`,
        {
          method: "PUT",
          body: chunk,
          signal,
        }
      );
      return response.ok;
    } catch (error) {
      if ((error as Error).name === "AbortError") {
        return false;
      }
      throw error;
    }
  };

  const completeUpload = async (uploadId: string): Promise<string | null> => {
    try {
      const response = await fetch(`${API_BASE}/upload/${uploadId}/complete`, {
        method: "POST",
      });

      if (!response.ok) {
        throw new Error("Failed to complete upload");
      }

      const data = await response.json();
      return data.video_id;
    } catch (error) {
      console.error("Complete upload error:", error);
      return null;
    }
  };

  const startUpload = async () => {
    const { file } = uploadState;
    if (!file) return;

    isPausedRef.current = false;
    abortControllerRef.current = new AbortController();

    setUploadState((prev) => ({ ...prev, status: "uploading", error: null }));

    // Initialize or resume session
    let session = sessionRef.current;
    let startChunk = 0;

    if (!session) {
      session = await initUpload(file);
      if (!session) {
        setUploadState((prev) => ({
          ...prev,
          status: "error",
          error: "Failed to initialize upload",
        }));
        return;
      }
      sessionRef.current = session;
    } else {
      // Resume from where we left off
      startChunk = uploadState.uploadedChunks;
    }

    const { uploadId, totalChunks, chunkSize } = session;
    let uploadedChunks = startChunk;
    let lastTime = Date.now();
    let lastBytes = startChunk * chunkSize;

    // Upload chunks sequentially
    for (let i = startChunk; i < totalChunks; i++) {
      if (isPausedRef.current) {
        setUploadState((prev) => ({ ...prev, status: "paused" }));
        return;
      }

      const start = i * chunkSize;
      const end = Math.min(start + chunkSize, file.size);
      const chunk = file.slice(start, end);

      try {
        const success = await uploadChunk(
          uploadId,
          i,
          chunk,
          abortControllerRef.current!.signal
        );

        if (!success) {
          if (isPausedRef.current) {
            setUploadState((prev) => ({ ...prev, status: "paused" }));
            return;
          }
          throw new Error(`Failed to upload chunk ${i}`);
        }

        uploadedChunks++;
        const progress = Math.round((uploadedChunks / totalChunks) * 100);

        // Calculate speed
        const now = Date.now();
        const timeDiff = (now - lastTime) / 1000;
        const bytesDiff = uploadedChunks * chunkSize - lastBytes;
        const speed =
          timeDiff > 0 ? formatSpeed(bytesDiff / timeDiff) : uploadState.speed;

        lastTime = now;
        lastBytes = uploadedChunks * chunkSize;

        setUploadState((prev) => ({
          ...prev,
          uploadedChunks,
          progress,
          speed,
        }));
      } catch (error) {
        if ((error as Error).name !== "AbortError") {
          setUploadState((prev) => ({
            ...prev,
            status: "error",
            error: `Upload failed at chunk ${i}: ${(error as Error).message}`,
          }));
        }
        return;
      }
    }

    // All chunks uploaded, complete the upload
    setUploadState((prev) => ({ ...prev, status: "processing", progress: 100 }));

    const videoId = await completeUpload(uploadId);
    if (videoId) {
      setUploadState((prev) => ({
        ...prev,
        status: "encoding",
        videoId,
      }));
      // Start polling for encoding status
      pollEncodingStatus(videoId);
    } else {
      setUploadState((prev) => ({
        ...prev,
        status: "error",
        error: "Failed to complete upload",
      }));
    }
  };

  const pauseUpload = () => {
    isPausedRef.current = true;
    abortControllerRef.current?.abort();
    setUploadState((prev) => ({ ...prev, status: "paused" }));
  };

  const resumeUpload = () => {
    startUpload();
  };

  const cancelUpload = async () => {
    isPausedRef.current = true;
    abortControllerRef.current?.abort();

    if (sessionRef.current) {
      try {
        await fetch(`${API_BASE}/upload/${sessionRef.current.uploadId}`, {
          method: "DELETE",
        });
      } catch (error) {
        console.error("Cancel upload error:", error);
      }
    }

    sessionRef.current = null;
    setUploadState({
      file: null,
      status: "idle",
      progress: 0,
      uploadedChunks: 0,
      totalChunks: 0,
      videoId: null,
      error: null,
      speed: "",
    });

    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  };

  const formatFileSize = (bytes: number): string => {
    if (bytes === 0) return "0 Bytes";
    const k = 1024;
    const sizes = ["Bytes", "KB", "MB", "GB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
  };

  const formatSpeed = (bytesPerSecond: number): string => {
    if (bytesPerSecond === 0) return "0 B/s";
    const k = 1024;
    const sizes = ["B/s", "KB/s", "MB/s", "GB/s"];
    const i = Math.floor(Math.log(bytesPerSecond) / Math.log(k));
    return (
      parseFloat((bytesPerSecond / Math.pow(k, i)).toFixed(2)) + " " + sizes[i]
    );
  };

  const formatETA = (seconds: number): string => {
    if (seconds < 60) return `${Math.round(seconds)}s`;
    if (seconds < 3600) {
      const mins = Math.floor(seconds / 60);
      const secs = Math.round(seconds % 60);
      return `${mins}m ${secs}s`;
    }
    const hours = Math.floor(seconds / 3600);
    const mins = Math.floor((seconds % 3600) / 60);
    return `${hours}h ${mins}m`;
  };

  const { file, status, progress, uploadedChunks, totalChunks, videoId, error, speed } =
    uploadState;

  return (
    <div className={styles.container}>
      <h1 className={styles.title}>Upload Video</h1>

      <div
        className={`${styles.dropzone} ${isDragging ? styles.dragging : ""} ${
          file ? styles.hasFile : ""
        }`}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => fileInputRef.current?.click()}
      >
        <input
          ref={fileInputRef}
          type="file"
          accept="video/*"
          onChange={handleFileSelect}
          className={styles.fileInput}
        />

        {!file ? (
          <div className={styles.dropzoneContent}>
            <svg
              className={styles.uploadIcon}
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
            >
              <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
              <polyline points="17 8 12 3 7 8" />
              <line x1="12" y1="3" x2="12" y2="15" />
            </svg>
            <p className={styles.dropzoneText}>
              Drag and drop your video here, or click to browse
            </p>
            <p className={styles.dropzoneHint}>Supports MP4, MOV, AVI, MKV</p>
          </div>
        ) : (
          <div className={styles.fileInfo}>
            <svg
              className={styles.videoIcon}
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
            >
              <rect x="2" y="2" width="20" height="20" rx="2.18" ry="2.18" />
              <line x1="7" y1="2" x2="7" y2="22" />
              <line x1="17" y1="2" x2="17" y2="22" />
              <line x1="2" y1="12" x2="22" y2="12" />
              <line x1="2" y1="7" x2="7" y2="7" />
              <line x1="2" y1="17" x2="7" y2="17" />
              <line x1="17" y1="17" x2="22" y2="17" />
              <line x1="17" y1="7" x2="22" y2="7" />
            </svg>
            <div className={styles.fileDetails}>
              <p className={styles.fileName}>{file.name}</p>
              <p className={styles.fileSize}>{formatFileSize(file.size)}</p>
            </div>
          </div>
        )}
      </div>

      {file && status !== "complete" && status !== "encoding" && (
        <div className={styles.progressSection}>
          <div className={styles.progressBar}>
            <div
              className={styles.progressFill}
              style={{ width: `${progress}%` }}
            />
          </div>
          <div className={styles.progressInfo}>
            <span>
              {progress}% ({uploadedChunks}/{totalChunks} chunks)
            </span>
            {speed && status === "uploading" && <span>{speed}</span>}
          </div>
        </div>
      )}

      {error && <div className={styles.error}>{error}</div>}

      {status === "processing" && (
        <div className={styles.processing}>
          <div className={styles.spinner} />
          <span>Processing video for streaming...</span>
        </div>
      )}

      {status === "encoding" && (
        <div className={styles.progressSection}>
          <div className={styles.progressBar}>
            <div
              className={styles.progressFill}
              style={{ width: `${encodingProgress?.progress ?? 0}%` }}
            />
          </div>
          <div className={styles.progressInfo}>
            <span>
              Encoding: {Math.round(encodingProgress?.progress ?? 0)}%
            </span>
            {encodingProgress?.speed && encodingProgress.speed > 0 && (
              <span>{encodingProgress.speed.toFixed(1)}x speed</span>
            )}
          </div>
          {encodingProgress?.eta_seconds && encodingProgress.eta_seconds > 0 && (
            <div className={styles.progressInfo}>
              <span>ETA: {formatETA(encodingProgress.eta_seconds)}</span>
            </div>
          )}
        </div>
      )}

      {status === "complete" && videoId && (
        <div className={styles.complete}>
          <svg
            className={styles.checkIcon}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
          >
            <polyline points="20 6 9 17 4 12" />
          </svg>
          <span>Upload complete!</span>
          <a
            href={`/?v=${videoId}`}
            className={styles.watchLink}
          >
            Watch Video
          </a>
        </div>
      )}

      <div className={styles.actions}>
        {status === "idle" && file && (
          <button className={styles.uploadButton} onClick={startUpload}>
            Start Upload
          </button>
        )}

        {status === "uploading" && (
          <button className={styles.pauseButton} onClick={pauseUpload}>
            Pause
          </button>
        )}

        {status === "paused" && (
          <>
            <button className={styles.resumeButton} onClick={resumeUpload}>
              Resume
            </button>
            <button className={styles.cancelButton} onClick={cancelUpload}>
              Cancel
            </button>
          </>
        )}

        {(status === "error" || status === "complete") && (
          <button className={styles.newUploadButton} onClick={cancelUpload}>
            New Upload
          </button>
        )}
      </div>
    </div>
  );
}
