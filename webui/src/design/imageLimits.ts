export const MAX_IMAGES = 10;
export const MAX_IMAGE_BYTES = 20 * 1024 * 1024;

export const MSG_MAX_IMAGES = "Max 10 images";
export const MSG_TOO_LARGE = "Too large, max 20 MB";
export const MSG_BAD_TYPE = "Not a supported image type";

function startsWith(b: Uint8Array, sig: number[], at = 0): boolean {
  return sig.every((v, i) => b[at + i] === v);
}

/** PNG, JPEG, GIF or WebP decided from the file's first bytes; the name and MIME type are ignored. */
export function isSupportedImageBytes(b: Uint8Array): boolean {
  return (
    startsWith(b, [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]) ||
    startsWith(b, [0xff, 0xd8, 0xff]) ||
    startsWith(b, [0x47, 0x49, 0x46, 0x38]) ||
    (startsWith(b, [0x52, 0x49, 0x46, 0x46]) && startsWith(b, [0x57, 0x45, 0x42, 0x50], 8))
  );
}

async function headBytes(file: File): Promise<Uint8Array> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(new Uint8Array(reader.result as ArrayBuffer));
    reader.onerror = () => reject(reader.error);
    reader.readAsArrayBuffer(file.slice(0, 12));
  });
}

export interface ImageCheck {
  accepted: File[];
  /** The first refusal message, or "" when every file passed. */
  error: string;
}

/** Runs before any upload. `existing` counts the images already in the draft. */
export async function checkImages(files: File[], existing: number): Promise<ImageCheck> {
  const accepted: File[] = [];
  let error = "";
  for (const file of files) {
    if (existing + accepted.length >= MAX_IMAGES) {
      error = error || MSG_MAX_IMAGES;
      break;
    }
    if (file.size > MAX_IMAGE_BYTES) {
      error = error || MSG_TOO_LARGE;
    } else if (!isSupportedImageBytes(await headBytes(file))) {
      error = error || MSG_BAD_TYPE;
    } else {
      accepted.push(file);
    }
  }
  return { accepted, error };
}
