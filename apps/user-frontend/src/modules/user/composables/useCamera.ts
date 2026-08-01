import { ref, onUnmounted, type Ref } from 'vue';

export interface UseCameraOptions {
  facingMode?: 'user' | 'environment';
  width?: number;
  height?: number;
}

export interface UseCameraReturn {
  stream: Ref<MediaStream | null>;
  error: Ref<string | null>;
  isActive: Ref<boolean>;
  start: (videoElement: HTMLVideoElement) => Promise<void>;
  stop: () => void;
  captureFrame: (videoElement: HTMLVideoElement, quality?: number) => Promise<Blob | null>;
}

export function useCamera(options: UseCameraOptions = {}): UseCameraReturn {
  const {
    facingMode = 'user',
    width = 1280,
    height = 720,
  } = options;

  const stream = ref<MediaStream | null>(null);
  const error = ref<string | null>(null);
  const isActive = ref(false);

  async function start(videoElement: HTMLVideoElement): Promise<void> {
    error.value = null;

    try {
      const mediaStream = await navigator.mediaDevices.getUserMedia({
        video: {
          facingMode,
          width: { ideal: width },
          height: { ideal: height },
        },
        audio: false,
      });

      stream.value = mediaStream;
      videoElement.srcObject = mediaStream;
      isActive.value = true;

      await new Promise<void>((resolve) => {
        videoElement.onloadedmetadata = () => {
          videoElement.play();
          resolve();
        };
      });
    } catch (err: unknown) {
      isActive.value = false;
      if (err instanceof DOMException) {
        if (err.name === 'NotAllowedError') {
          error.value = 'camera_denied';
        } else if (err.name === 'NotFoundError') {
          error.value = 'camera_not_found';
        } else {
          error.value = 'camera_error';
        }
      } else {
        error.value = 'camera_error';
      }
    }
  }

  function stop(): void {
    if (stream.value) {
      stream.value.getTracks().forEach((track) => track.stop());
      stream.value = null;
    }
    isActive.value = false;
  }

  async function captureFrame(
    videoElement: HTMLVideoElement,
    quality: number = 0.85
  ): Promise<Blob | null> {
    if (!isActive.value || videoElement.videoWidth === 0) {
      return null;
    }

    const canvas = document.createElement('canvas');
    canvas.width = videoElement.videoWidth;
    canvas.height = videoElement.videoHeight;

    const ctx = canvas.getContext('2d');
    if (!ctx) return null;

    ctx.drawImage(videoElement, 0, 0);

    return new Promise<Blob | null>((resolve) => {
      canvas.toBlob(
        (blob) => resolve(blob),
        'image/jpeg',
        quality
      );
    });
  }

  onUnmounted(() => {
    stop();
  });

  return {
    stream,
    error,
    isActive,
    start,
    stop,
    captureFrame,
  };
}
