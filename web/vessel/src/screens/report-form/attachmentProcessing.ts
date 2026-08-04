// SPDX-License-Identifier: AGPL-3.0-only

// Client-side attachment processing (architecture 15): images are
// downscaled and re-encoded (WebP, falling back to JPEG) targeting under
// 500 KB before upload; PDFs pass through unmodified with a soft warning
// above 2 MB and a hard cap at 5 MB. Runs entirely in the browser via
// Canvas — no server round-trip needed just to reject an oversized file
// or shrink a camera photo.

export const IMAGE_TARGET_BYTES = 500 * 1024;
export const PDF_SOFT_WARN_BYTES = 2 * 1024 * 1024;
export const PDF_HARD_CAP_BYTES = 5 * 1024 * 1024;

// Longest-side cap applied before quality reduction even starts — a
// full-resolution phone photo (commonly 4000px+) wastes encode time and
// still needs heavy downscaling to hit 500 KB anyway; capping the
// starting dimension up front makes the quality-reduction loop below
// converge in fewer steps without visibly hurting a document scan's
// legibility.
const MAX_IMAGE_DIMENSION = 2000;

export interface ProcessedAttachment {
  blob: Blob;
  filename: string;
  contentType: string;
  originalSize: number;
  finalSize: number;
  /** Set when the file is accepted but the officer should know something (PDF over the soft-warn threshold). */
  warning?: string;
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(0)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

/** Throws with a user-facing message if the file can't be attached at all (wrong type, PDF over the hard cap). */
export async function processAttachmentFile(file: File): Promise<ProcessedAttachment> {
  if (file.type === "application/pdf") {
    return processPdf(file);
  }
  if (file.type.startsWith("image/")) {
    return processImage(file);
  }
  throw new Error("Only images and PDFs can be attached.");
}

function processPdf(file: File): ProcessedAttachment {
  if (file.size > PDF_HARD_CAP_BYTES) {
    throw new Error(`This PDF is ${formatBytes(file.size)}, over the ${formatBytes(PDF_HARD_CAP_BYTES)} limit.`);
  }
  const warning =
    file.size > PDF_SOFT_WARN_BYTES
      ? `This PDF is ${formatBytes(file.size)} — consider a smaller scan if possible.`
      : undefined;
  return { blob: file, filename: file.name, contentType: file.type, originalSize: file.size, finalSize: file.size, warning };
}

const QUALITY_STEPS = [0.85, 0.7, 0.55, 0.4];
const MAX_DIMENSION_PASSES = 4;
const DIMENSION_SHRINK_FACTOR = 0.75;

function canvasToBlob(canvas: HTMLCanvasElement, type: string, quality: number): Promise<Blob | null> {
  return new Promise((resolve) => canvas.toBlob(resolve, type, quality));
}

async function processImage(file: File): Promise<ProcessedAttachment> {
  const bitmap = await createImageBitmap(file);
  let width = bitmap.width;
  let height = bitmap.height;
  if (width > MAX_IMAGE_DIMENSION || height > MAX_IMAGE_DIMENSION) {
    const scale = MAX_IMAGE_DIMENSION / Math.max(width, height);
    width = Math.round(width * scale);
    height = Math.round(height * scale);
  }

  const canvas = document.createElement("canvas");
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("This browser can't process images for attachment.");

  let best: Blob | null = null;
  let contentType = "image/webp";
  for (let pass = 0; pass < MAX_DIMENSION_PASSES; pass++) {
    canvas.width = width;
    canvas.height = height;
    ctx.clearRect(0, 0, width, height);
    ctx.drawImage(bitmap, 0, 0, width, height);

    for (const quality of QUALITY_STEPS) {
      let blob = await canvasToBlob(canvas, "image/webp", quality);
      contentType = "image/webp";
      if (!blob || blob.type !== "image/webp") {
        // WebP encode unsupported on this browser — JPEG is universally
        // supported by every canvas.toBlob implementation.
        blob = await canvasToBlob(canvas, "image/jpeg", quality);
        contentType = "image/jpeg";
      }
      if (blob && (!best || blob.size < best.size)) best = blob;
      if (blob && blob.size <= IMAGE_TARGET_BYTES) {
        return finishImage(best!, contentType, file);
      }
    }
    width = Math.round(width * DIMENSION_SHRINK_FACTOR);
    height = Math.round(height * DIMENSION_SHRINK_FACTOR);
  }
  // Never hit the target — accept the smallest encoding produced rather
  // than looping forever or blocking the upload outright.
  if (!best) throw new Error("Could not re-encode this image.");
  return finishImage(best, contentType, file);
}

function finishImage(blob: Blob, contentType: string, original: File): ProcessedAttachment {
  const ext = contentType === "image/webp" ? "webp" : "jpg";
  const base = original.name.replace(/\.[^./]+$/, "");
  return {
    blob,
    filename: `${base}.${ext}`,
    contentType,
    originalSize: original.size,
    finalSize: blob.size,
  };
}
