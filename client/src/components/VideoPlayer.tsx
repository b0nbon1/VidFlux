import React, { useEffect, useRef } from "react";
import videojs from "video.js";
import "videojs-hls-quality-selector";
import "video.js/dist/video-js.css";
import httpSourceSelector from "videojs-http-source-selector";

interface VideoPlayerProps {
  src: string;
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type VideoJsPlayer = ReturnType<typeof videojs> & { [key: string]: any };

const VideoPlayer = ({ src }: VideoPlayerProps) => {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const playerRef = useRef<VideoJsPlayer | null>(null);

  useEffect(() => {
    if (!playerRef.current && videoRef.current) {
      playerRef.current = videojs(videoRef.current, {
        controls: true,
        autoplay: true,
        responsive: true,
        fluid: true,
        html5: {
          vhs: {
            enableLowInitialPlaylist: true,
            smoothQualityChange: true,
            fastQualityChange: true,
            handlePartialData: true,
            llhls: false,
          },
          hls: {
            overrideNative: true,
            limitRenditionByPlayerDimensions: true,
            useDevicePixelRatio: true,
          },
          nativeAudioTracks: false,
          nativeVideoTracks: false,
          useBandwidthFromLocalStorage: true,
        },
        liveTracker: {
          trackingThreshold: 0,
          liveTolerance: Infinity,
        },
        controlBar: {
          pictureInPictureToggle: false,
        },
      }) as VideoJsPlayer;

      // Register the quality selector plugin
      videojs.registerPlugin("httpSourceSelector", httpSourceSelector);
      playerRef.current.httpSourceSelector();

      // Load HLS source
      playerRef.current.src({
        src: src,
        type: "application/x-mpegURL",
      });

      const player = playerRef.current;
      
      player.ready(() => {
        player.hlsQualitySelector({
          displayCurrentQuality: true,
        });
      });
    }

    return () => {
      if (playerRef.current) {
        playerRef.current.dispose();
        playerRef.current = null;
      }
    };
  }, [src]);

  return (
    <div style={{ width: "640px", height: "480px" }}>
    <div data-vjs-player>
      <video ref={videoRef} className="video-js vjs-default-skin" />
    </div>
    </div>
  );
};

export default VideoPlayer;
