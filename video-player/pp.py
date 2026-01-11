#!/usr/bin/env python3
"""
Simple Python-based CLI video player (pp - python player)
"""

import argparse
import cv2
import os
import sys
import json
import time
import threading
import subprocess
import signal
import logging
import numpy as np
from pathlib import Path
from typing import List, Dict, Optional

__version__ = "0.1.0"

class VideoPlayer:
    def __init__(self, path: str, seek_short: int = 10, seek_long: int = 60, throttle_delay: float = 0.2, continuous: bool = False, latest: bool = False):
        self.path = Path(path)
        self.seek_short = seek_short  # seconds
        self.seek_long = seek_long    # seconds
        self.throttle_delay = throttle_delay  # seconds
        self.continuous = continuous  # auto-advance to next video
        self.latest = latest  # sort by modification time
        self.video_files = []
        self.current_index = 0
        self.is_playing = True
        self.is_muted = False
        self.timestamps = {}  # Store last watched timestamps
        self.cap = None
        self.fps = 30
        self.frame_count = 0
        self.current_frame = 0
        self.playback_speed = 1.0  # Default playback speed

        # Status display
        self.status_text = ""
        self.status_start_time = 0
        self.status_duration = 2.5  # Show status for 2.5 seconds
        self.available_fonts = self.get_available_fonts()

        # Audio support using ffplay
        self.audio_process = None

        # Throttling for seek operations
        self.pending_seek_operations = []
        self.seek_throttle_timer = None
        self.execute_pending_seek = False  # Flag to execute seeks in main thread

        # Setup logging first (needed by other methods)
        self.setup_logging()

        # Check audio availability after logging is set up
        self.audio_available = self.check_ffplay_available()

        # Supported video extensions
        self.video_extensions = {'.mp4', '.avi', '.mkv', '.mov', '.wmv', '.flv', '.webm', '.m4v'}

        # Load video files
        self.load_video_files()

        # Load saved timestamps
        self.load_timestamps()

    def setup_logging(self):
        """Setup logging configuration"""
        logging.basicConfig(
            level=logging.INFO,
            format='%(asctime)s - %(levelname)s - %(message)s',
            handlers=[
                logging.FileHandler(Path.home() / '.pp_player.log'),
                logging.StreamHandler(sys.stdout)
            ]
        )
        self.logger = logging.getLogger(__name__)

    def show_status(self, message: str):
        """Show status message on screen for a few seconds"""
        self.status_text = message
        self.status_start_time = time.time()
        self.logger.info(message)

    def get_available_fonts(self):
        """Get list of available fonts with fallback options"""
        # List of fonts to try in order of preference
        font_options = [
            cv2.FONT_HERSHEY_SIMPLEX,
            cv2.FONT_HERSHEY_PLAIN,
            cv2.FONT_HERSHEY_DUPLEX,
            cv2.FONT_HERSHEY_COMPLEX,
            cv2.FONT_HERSHEY_TRIPLEX,
            cv2.FONT_HERSHEY_COMPLEX_SMALL,
            cv2.FONT_HERSHEY_SCRIPT_SIMPLEX,
            cv2.FONT_HERSHEY_SCRIPT_COMPLEX
        ]

        # Test each font to see which ones work
        available = []
        test_frame = np.zeros((100, 300, 3), dtype=np.uint8)

        for font in font_options:
            try:
                # Try to get text size and put text
                size = cv2.getTextSize("Test", font, 1.0, 2)
                if size[0][0] > 0 and size[0][1] > 0:  # Valid size
                    cv2.putText(test_frame, "Test", (10, 30), font, 1.0, (255, 255, 255), 2)
                    available.append(font)
            except Exception:
                continue

        # If no fonts work, fallback to SIMPLEX
        if not available:
            available = [cv2.FONT_HERSHEY_SIMPLEX]

        return available

    def get_working_font(self):
        """Get the first working font from available fonts"""
        return self.available_fonts[0] if self.available_fonts else cv2.FONT_HERSHEY_SIMPLEX

    def draw_status_overlay(self, frame):
        """Draw status overlay on frame if active"""
        if not self.status_text:
            return frame

        current_time = time.time()
        elapsed = current_time - self.status_start_time

        if elapsed > self.status_duration:
            self.status_text = ""
            return frame

        # Calculate fade effect (fade out in last 0.5 seconds)
        fade_start = self.status_duration - 0.5
        if elapsed > fade_start:
            alpha = 1.0 - ((elapsed - fade_start) / 0.5)
        else:
            alpha = 1.0

        # Create overlay
        overlay = frame.copy()
        height, width = frame.shape[:2]

        # Get working font
        font = self.get_working_font()
        font_scale = 0.8
        thickness = 2

        # Try to get text size with fallback
        try:
            text_size = cv2.getTextSize(self.status_text, font, font_scale, thickness)[0]
        except Exception:
            # Fallback if getTextSize fails
            self.logger.warning("Font rendering issue, using fallback")
            font = cv2.FONT_HERSHEY_PLAIN
            font_scale = 1.0
            try:
                text_size = cv2.getTextSize(self.status_text, font, font_scale, thickness)[0]
            except Exception:
                # Ultimate fallback - estimate size
                text_size = (len(self.status_text) * 12, 20)

        # Position at top-left with padding
        padding = 15
        text_x = padding
        text_y = padding + text_size[1]

        # Semi-transparent background rectangle
        bg_x1 = text_x - 10
        bg_y1 = text_y - text_size[1] - 5
        bg_x2 = text_x + text_size[0] + 10
        bg_y2 = text_y + 10

        # Ensure background doesn't go outside frame
        bg_x1 = max(0, bg_x1)
        bg_y1 = max(0, bg_y1)
        bg_x2 = min(width, bg_x2)
        bg_y2 = min(height, bg_y2)

        try:
            # Draw semi-transparent background
            cv2.rectangle(overlay, (bg_x1, bg_y1), (bg_x2, bg_y2), (0, 0, 0), -1)

            # Draw status text
            cv2.putText(overlay, self.status_text, (text_x, text_y), 
                       font, font_scale, (255, 255, 255), thickness)
        except Exception as e:
            # If drawing fails, log but don't crash
            self.logger.warning(f"Failed to draw status overlay: {e}")
            return frame

        # Blend with alpha for fade effect
        try:
            frame = cv2.addWeighted(frame, 1 - alpha * 0.8, overlay, alpha * 0.8, 0)
        except Exception:
            # If blending fails, return original frame
            return frame

        return frame

    def load_video_files(self):
        """Load all video files from the given path"""
        if self.path.is_file():
            # If it's a file, get all videos from its directory
            directory = self.path.parent
        else:
            # If it's a directory, use it directly
            directory = self.path

        if not directory.exists():
            self.logger.error(f"Path {directory} does not exist")
            sys.exit(1)

        # Find all video files
        for file_path in directory.iterdir():
            if file_path.is_file() and file_path.suffix.lower() in self.video_extensions:
                self.video_files.append(file_path)

        # Sort files for consistent ordering
        self.video_files.sort()

        if not self.video_files:
            self.logger.error(f"No video files found in {directory}")
            sys.exit(1)

        # If a specific file was provided, set it as current
        if self.path.is_file() and self.path in self.video_files:
            self.current_index = self.video_files.index(self.path)

        self.logger.info(f"Found {len(self.video_files)} video files")

    def load_timestamps(self):
        """Load saved timestamps from file"""
        timestamp_file = Path.home() / '.pp_timestamps.json'
        if timestamp_file.exists():
            try:
                with open(timestamp_file, 'r') as f:
                    self.timestamps = json.load(f)
            except:
                self.timestamps = {}

    def save_timestamps(self):
        """Save current timestamps to file"""
        timestamp_file = Path.home() / '.pp_timestamps.json'
        try:
            with open(timestamp_file, 'w') as f:
                json.dump(self.timestamps, f)
        except:
            pass

    def check_ffplay_available(self):
        """Check if ffplay is available on the system"""
        try:
            subprocess.run(['ffplay', '-version'], stdout=subprocess.DEVNULL, 
                         stderr=subprocess.DEVNULL, check=True)
            return True
        except (subprocess.CalledProcessError, FileNotFoundError):
            self.logger.warning("ffplay not found. Audio playback disabled.")
            self.logger.info("Install ffmpeg to enable audio: https://ffmpeg.org/download.html")
            return False

    def stop_audio(self):
        """Stop current audio playback"""
        if self.audio_process and self.audio_process.poll() is None:
            try:
                # Force kill the process group to prevent threading issues
                if hasattr(os, 'killpg'):
                    os.killpg(os.getpgid(self.audio_process.pid), signal.SIGKILL)
                else:
                    self.audio_process.kill()
                self.audio_process.wait(timeout=1)
            except (subprocess.TimeoutExpired, ProcessLookupError, OSError):
                # Process already dead or can't be killed
                pass
            except Exception as e:
                self.logger.warning(f"Error stopping audio: {e}")
        self.audio_process = None

    def play_audio(self, video_path: Path, start_time: float = 0):
        """Play audio for the given video file using ffplay"""
        if not self.audio_available:
            return

        try:
            # Stop any existing audio
            self.stop_audio()

            # Small delay to ensure process is fully terminated
            time.sleep(0.1)

            # Build ffplay command
            cmd = [
                'ffplay',
                '-nodisp',  # No video display
                '-autoexit',  # Exit when playback finishes
                '-loglevel', 'quiet',  # Suppress ffplay output
            ]

            # Add start time if specified
            if start_time > 0:
                cmd.extend(['-ss', str(start_time)])

            # Add audio filters for volume and speed
            audio_filters = []
            if self.is_muted:
                audio_filters.append('volume=0')
            if self.playback_speed != 1.0:
                audio_filters.append(f'atempo={self.playback_speed}')

            if audio_filters:
                cmd.extend(['-af', ','.join(audio_filters)])

            cmd.append(str(video_path))

            # Start audio process
            self.audio_process = subprocess.Popen(
                cmd,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                preexec_fn=os.setsid if hasattr(os, 'setsid') else None
            )

        except Exception as e:
            self.logger.warning(f"Could not start audio for {video_path.name}: {e}")

    def pause_audio(self):
        """Pause audio playback"""
        if self.audio_process and self.audio_process.poll() is None:
            try:
                # Send SIGSTOP to pause
                if hasattr(os, 'killpg'):
                    os.killpg(os.getpgid(self.audio_process.pid), signal.SIGSTOP)
                else:
                    self.audio_process.send_signal(signal.SIGSTOP)
            except:
                pass

    def resume_audio(self):
        """Resume audio playback"""
        if self.audio_process and self.audio_process.poll() is None:
            try:
                # Send SIGCONT to resume
                if hasattr(os, 'killpg'):
                    os.killpg(os.getpgid(self.audio_process.pid), signal.SIGCONT)
                else:
                    self.audio_process.send_signal(signal.SIGCONT)
            except:
                pass

    def set_audio_volume(self, muted: bool):
        """Set audio volume based on mute state"""
        # For simplicity, we restart audio with new volume
        # In a more advanced implementation, we could use ffmpeg filters
        if self.audio_process:
            current_video = self.video_files[self.current_index]
            current_time = self.cap.get(cv2.CAP_PROP_POS_MSEC) / 1000.0 if self.cap else 0
            self.stop_audio()
            self.play_audio(current_video, current_time)

    def load_video(self, index: int):
        """Load video at given index"""
        if 0 <= index < len(self.video_files):
            # Save current timestamp before switching
            if self.cap is not None:
                current_time = self.cap.get(cv2.CAP_PROP_POS_MSEC) / 1000.0
                self.timestamps[str(self.video_files[self.current_index])] = current_time

            # Stop current audio
            self.stop_audio()

            self.current_index = index
            video_path = self.video_files[self.current_index]

            if self.cap:
                self.cap.release()

            self.cap = cv2.VideoCapture(str(video_path))
            if not self.cap.isOpened():
                self.logger.error(f"Cannot open video {video_path}")
                return False

            self.fps = self.cap.get(cv2.CAP_PROP_FPS)
            self.frame_count = int(self.cap.get(cv2.CAP_PROP_FRAME_COUNT))

            # Restore timestamp if available
            saved_time = self.timestamps.get(str(video_path), 0)
            if saved_time > 0:
                self.cap.set(cv2.CAP_PROP_POS_MSEC, saved_time * 1000)

            # Start audio playback
            self.play_audio(video_path, saved_time)

            self.logger.info(f"Playing: {video_path.name} ({self.current_index + 1}/{len(self.video_files)})")
            self.show_status(f"Playing: {video_path.name}")

            # Update window title with video name
            self.update_window_title(video_path.name)
            return True
        return False

    def seek(self, seconds: float):
        """Seek forward or backward by given seconds"""
        if self.cap is None:
            return

        current_time = self.cap.get(cv2.CAP_PROP_POS_MSEC) / 1000.0
        new_time = max(0, current_time + seconds)

        # Don't seek beyond video duration
        duration = self.frame_count / self.fps
        new_time = min(new_time, duration)

        # Update video position
        self.cap.set(cv2.CAP_PROP_POS_MSEC, new_time * 1000)

        # Restart audio from new position to maintain sync
        if self.audio_available and self.audio_process:
            current_video = self.video_files[self.current_index]
            self.stop_audio()
            self.play_audio(current_video, new_time)

    def seek_to_position(self, position: float):
        """Seek to absolute position in seconds"""
        if self.cap is None:
            return

        # Don't seek beyond video duration
        duration = self.frame_count / self.fps
        position = max(0, min(position, duration))

        # Update video position
        self.cap.set(cv2.CAP_PROP_POS_MSEC, position * 1000)

        # Restart audio from new position to maintain sync
        if self.audio_available and self.audio_process:
            current_video = self.video_files[self.current_index]
            self.stop_audio()
            self.play_audio(current_video, position)

    def throttled_seek(self, seconds: float):
        """Add seek operation to throttle queue"""
        # Add to pending operations
        self.pending_seek_operations.append(seconds)

        # Cancel existing timer if any
        if self.seek_throttle_timer:
            self.seek_throttle_timer.cancel()

        # Stop audio immediately to prevent assertion errors during rapid seeking
        self.stop_audio()

        # Start new timer to set flag for main thread execution
        self.seek_throttle_timer = threading.Timer(self.throttle_delay, self.set_execute_seek_flag)
        self.seek_throttle_timer.start()

    def set_execute_seek_flag(self):
        """Set flag for main thread to execute pending seeks"""
        self.execute_pending_seek = True

    def execute_throttled_seeks(self):
        """Execute accumulated seek operations"""
        if not self.pending_seek_operations:
            return

        # Calculate total seek amount
        total_seek = sum(self.pending_seek_operations)

        # Apply throttling if too many operations
        if len(self.pending_seek_operations) >= 3:
            # For 3+ operations, use 5x multiplier for efficiency
            multiplier = 5.0
            total_seek *= multiplier
            self.show_status(f"Fast seek: {total_seek:.0f}s ({len(self.pending_seek_operations)} ops)")
        else:
            # For 1-2 operations, show normal seek
            direction = "▶" if total_seek > 0 else "◀"
            self.show_status(f"{direction} {abs(total_seek):.0f}s")

        # Clear pending operations
        self.pending_seek_operations.clear()
        self.execute_pending_seek = False

        # Execute the accumulated seek (safe from main thread)
        if self.cap is None:
            return

        current_time = self.cap.get(cv2.CAP_PROP_POS_MSEC) / 1000.0
        new_time = max(0, current_time + total_seek)

        # Don't seek beyond video duration
        duration = self.frame_count / self.fps
        new_time = min(new_time, duration)

        # Update video position
        self.cap.set(cv2.CAP_PROP_POS_MSEC, new_time * 1000)

        # Restart audio from new position
        if self.audio_available:
            current_video = self.video_files[self.current_index]
            self.play_audio(current_video, new_time)

    def change_playback_speed(self, delta: float):
        """Change playback speed by delta amount"""
        old_speed = self.playback_speed
        self.playback_speed = max(0.1, min(3.0, self.playback_speed + delta))

        if abs(self.playback_speed - old_speed) > 0.01:  # Only update if speed actually changed
            self.show_status(f"Speed: {self.playback_speed:.1f}x")

            # Restart audio with new speed to maintain sync
            if self.audio_available and self.audio_process:
                current_video = self.video_files[self.current_index]
                current_time = self.cap.get(cv2.CAP_PROP_POS_MSEC) / 1000.0 if self.cap else 0
                self.stop_audio()
                self.play_audio(current_video, current_time)

    def update_window_title(self, video_name: str):
        """Update window title with current video name"""
        speed_indicator = f" [{self.playback_speed:.1f}x]" if self.playback_speed != 1.0 else ""
        title = f"pp - {video_name} ({self.current_index + 1}/{len(self.video_files)}){speed_indicator}"
        cv2.setWindowTitle('Video Player', title)

    def play(self):
        """Main playback loop"""
        if not self.load_video(self.current_index):
            return

        cv2.namedWindow('Video Player', cv2.WINDOW_NORMAL)

        # Calculate proper delay based on FPS
        delay = int(1000 / self.fps) if self.fps > 0 else 33  # Default to ~30 FPS if fps is 0
        last_frame_time = time.time()

        while True:
            # Check for pending seek operations from timer thread
            if self.execute_pending_seek:
                self.execute_throttled_seeks()

            if self.is_playing:
                current_time = time.time()
                time_since_last_frame = current_time - last_frame_time

                # Only read new frame if enough time has passed (adjusted for speed)
                if time_since_last_frame >= (1.0 / (self.fps * self.playback_speed)):
                    ret, frame = self.cap.read()
                    if not ret:
                        # End of video
                        if self.continuous:
                            # Auto-advance to next video
                            self.next_video()
                            # Update delay for new video
                            delay = int(1000 / self.fps) if self.fps > 0 else 33
                            last_frame_time = time.time()
                            continue
                        else:
                            # Pause and wait for user input
                            self.is_playing = False
                            self.pause_audio()
                            self.show_status("Video ended - Press Enter for next video")
                            continue

                    # Add status overlay
                    frame_with_status = self.draw_status_overlay(frame)
                    cv2.imshow('Video Player', frame_with_status)
                    last_frame_time = current_time

            # Handle keyboard input with proper delay
            key = cv2.waitKey(1 if self.is_playing else 0) & 0xFF

            if key == ord('q') or key == 27:  # ESC key
                break
            elif key == ord(' '):  # Space - pause/play
                self.is_playing = not self.is_playing
                if self.is_playing:
                    self.resume_audio()
                    self.show_status("Playing")
                else:
                    self.pause_audio()
                    self.show_status("Paused")
            elif key == ord('m'):  # Mute toggle
                self.is_muted = not self.is_muted
                self.set_audio_volume(self.is_muted)
                self.show_status("Muted" if self.is_muted else "Unmuted")
            elif key == 81 or key == 2:  # Left arrow - seek backward (short)
                self.throttled_seek(-self.seek_short)
            elif key == 83 or key == 3:  # Right arrow - seek forward (short)
                self.throttled_seek(self.seek_short)
            elif key == 82 or key == 0:  # Up arrow - seek forward (long)
                self.throttled_seek(self.seek_long)
            elif key == 84 or key == 1:  # Down arrow - seek backward (long)
                self.throttled_seek(-self.seek_long)
            elif key == ord('s'):  # Start of video
                self.seek_to_position(0)
                self.show_status("Start of video")
            elif key == ord('e'):  # End of video
                duration = self.frame_count / self.fps
                self.seek_to_position(duration - 5)  # 5 seconds before end
                self.show_status("End of video")
            elif key == ord('j'):  # Previous video
                self.prev_video()
            elif key == ord('k') or key == 13:  # Next video (k or Enter)
                # Always allow next video, whether playing or paused
                self.next_video()
            elif key == ord(']'):  # Speed up 10%
                self.change_playback_speed(0.1)
                self.update_window_title(self.video_files[self.current_index].name)
            elif key == ord('['):  # Speed down 10%
                self.change_playback_speed(-0.1)
                self.update_window_title(self.video_files[self.current_index].name)

        # Cleanup
        if self.cap:
            # Save current timestamp
            current_time = self.cap.get(cv2.CAP_PROP_POS_MSEC) / 1000.0
            self.timestamps[str(self.video_files[self.current_index])] = current_time
            self.cap.release()

        # Cancel any pending seek operations
        if self.seek_throttle_timer:
            self.seek_throttle_timer.cancel()

        # Stop audio
        self.stop_audio()

        cv2.destroyAllWindows()
        self.save_timestamps()

    def next_video(self):
        """Switch to next video"""
        next_index = (self.current_index + 1) % len(self.video_files)
        self.load_video(next_index)

    def prev_video(self):
        """Switch to previous video"""
        prev_index = (self.current_index - 1) % len(self.video_files)
        self.load_video(prev_index)


def main():
    parser = argparse.ArgumentParser(description='Simple Python Video Player (pp)')
    parser.add_argument('path', nargs='?', default='.', 
                       help='Path to video file or directory (default: current directory)')
    parser.add_argument('--seek-short', type=int, default=10,
                       help='Short seek duration in seconds (default: 10)')
    parser.add_argument('--seek-long', type=int, default=60,
                       help='Long seek duration in seconds (default: 60)')
    parser.add_argument('--throttle-delay', type=float, default=0.2,
                       help='Throttle delay for rapid seeking in seconds (default: 0.2)')
    parser.add_argument('--continuous', action='store_true',
                       help='Continuous playing mode - auto-advance to next video (default: pause at end)')

    args = parser.parse_args()

    player = VideoPlayer(args.path, args.seek_short, args.seek_long, args.throttle_delay, args.continuous)

    logger = logging.getLogger(__name__)
    logger.info("Controls:")
    logger.info("  Space: Pause/Play")
    logger.info("  Left/Right: Seek ±10s (or custom)")
    logger.info("  Up/Down: Seek ±1min (or custom)")
    logger.info("  s: Jump to start of video")
    logger.info("  e: Jump to end of video")
    logger.info("  j: Previous video")
    logger.info("  k/Enter: Next video")
    logger.info("  m: Mute/Unmute")
    logger.info("  ]: Speed up 10% (max 3.0x)")
    logger.info("  [: Speed down 10% (min 0.1x)")
    logger.info("  q/ESC: Quit")
    if args.continuous:
        logger.info("\nContinuous mode: Videos will auto-advance")
    else:
        logger.info("\nPause mode: Videos will pause at end, press Enter for next video")
    print()
    print()

    try:
        player.play()
    except KeyboardInterrupt:
        logger.info("Exiting...")
    except Exception as e:
        logger.error(f"Error: {e}")


if __name__ == "__main__":
    main()