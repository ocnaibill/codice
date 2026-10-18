// The cutting of a cover (DEC-173). Every cover in the library is 2:3, so a picture is cut to that, in the browser, to 600×900, and what is
// sent is the cut (a JPEG). The geometry is here, free of the page, so it can be tested.

export const FRAME_WIDTH = 200;
export const FRAME_HEIGHT = 300;
export const OUTPUT_WIDTH = 600;
export const OUTPUT_HEIGHT = 900;
export const MAX_ZOOM = 4;

// What is cut: a cover (2:3) or the banner of a series (3:1, DEC-201). The frame is what the page shows; the output is what is sent.
export const COVER = Object.freeze({ frameWidth: FRAME_WIDTH, frameHeight: FRAME_HEIGHT, outputWidth: OUTPUT_WIDTH, outputHeight: OUTPUT_HEIGHT });
export const BANNER = Object.freeze({ frameWidth: 360, frameHeight: 120, outputWidth: 1500, outputHeight: 500 });

/** The scale at which the picture just covers the frame: no empty edge, whatever its proportion. */
export function baseScale(imageWidth, imageHeight, shape = COVER) {
  return Math.max(shape.frameWidth / imageWidth, shape.frameHeight / imageHeight);
}

/** Keeps the picture over the whole frame: its top-left corner can go no further right or down than the frame's, and no further
 *  left or up than what leaves the opposite edge on the frame's. */
export function clampOffset(offset, imageWidth, imageHeight, zoom, shape = COVER) {
  const scale = baseScale(imageWidth, imageHeight, shape) * zoom;
  const minX = shape.frameWidth - imageWidth * scale;
  const minY = shape.frameHeight - imageHeight * scale;
  return {
    x: Math.min(0, Math.max(minX, offset.x)),
    y: Math.min(0, Math.max(minY, offset.y)),
  };
}

/** The picture centred in the frame at the given zoom. */
export function centered(imageWidth, imageHeight, zoom = 1, shape = COVER) {
  const scale = baseScale(imageWidth, imageHeight, shape) * zoom;
  return clampOffset({ x: (shape.frameWidth - imageWidth * scale) / 2, y: (shape.frameHeight - imageHeight * scale) / 2 }, imageWidth, imageHeight, zoom, shape);
}

/** Changes the zoom keeping the same point of the picture at the centre of the frame. */
export function zoomAround(offset, imageWidth, imageHeight, from, to, shape = COVER) {
  const base = baseScale(imageWidth, imageHeight, shape);
  const centreX = (shape.frameWidth / 2 - offset.x) / (base * from);
  const centreY = (shape.frameHeight / 2 - offset.y) / (base * from);
  return clampOffset({ x: shape.frameWidth / 2 - centreX * base * to, y: shape.frameHeight / 2 - centreY * base * to }, imageWidth, imageHeight, to, shape);
}

/** The rectangle of the picture, in its own pixels, that the frame shows. */
export function sourceRect(offset, imageWidth, imageHeight, zoom, shape = COVER) {
  const scale = baseScale(imageWidth, imageHeight, shape) * zoom;
  return { x: (0 - offset.x) / scale, y: (0 - offset.y) / scale, width: shape.frameWidth / scale, height: shape.frameHeight / scale };
}

/** Cuts the picture to the output of the shape (600×900 for a cover, 1500×500 for a banner) and gives the JPEG. A transparent picture goes over white: a JPEG has no transparency. */
export function cutToBlob(image, offset, zoom, shape = COVER) {
  const rect = sourceRect(offset, image.naturalWidth, image.naturalHeight, zoom, shape);
  const canvas = document.createElement('canvas');
  canvas.width = shape.outputWidth;
  canvas.height = shape.outputHeight;
  const context = canvas.getContext('2d');
  context.fillStyle = '#ffffff';
  context.fillRect(0, 0, shape.outputWidth, shape.outputHeight);
  context.drawImage(image, rect.x, rect.y, rect.width, rect.height, 0, 0, shape.outputWidth, shape.outputHeight);
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('The cut could not be made'))), 'image/jpeg', 0.88);
  });
}
