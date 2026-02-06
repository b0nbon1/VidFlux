"use client";

import { useSearchParams } from "next/navigation";
import VideoPlayer from "./VideoPlayer";

function Player() {
  const searchParams = useSearchParams();
  const videoId = searchParams.get("v");

  if (!videoId) {
    return <p>No video ID provided. Use ?v=VIDEO_ID in the URL.</p>;
  }

  const videoSrc = `http://127.0.0.1:4500/static/videos/${videoId}/master.m3u8`;

  return (
    <>
      <h1>Video streaming app</h1>
      <VideoPlayer src={videoSrc} />
    </>
  );
}

export default Player;
