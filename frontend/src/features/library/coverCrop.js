// The cutting of a cover (DEC-173). Every cover in the library is 2:3, so a picture is cut to that, in the browser, to 600×900, and what is
// sent is the cut (a JPEG). The geometry is here, free of the page, so it can be tested.

export const FRAME_WIDTH = 200;
export const FRAME_HEIGHT = 300;
export const OUTPUT_WIDTH = 600;
export const OUTPUT_HEIGHT = 900;
export const MAX_ZOOM = 4;

/** The scale at which the picture just covers the frame: no empty edge, whatever its proportion. */
export function baseScale(imageWidth, imageHeight) {
  return Math.max(FRAME_WIDTH / imageWidth, FRAME_HEIGHT / imageHeight);
}

/** Keeps the picture over the whole frame: its top-left corner can go no further right or down than the frame's, and no further
 *  left or up than what leaves the opposite edge on the frame's. */
export function clampOffset(offset, imageWidth, imageHeight, zoom) {
  const scale = baseScale(imageWidth, imageHeight) * zoom;
  const minX = FRAME_WIDTH - imageWidth * scale;
  const minY = FRAME_HEIGHT - imageHeight * scale;
  return {
    x: Math.min(0, Math.max(minX, offset.x)),
    y: Math.min(0, Math.max(minY, offset.y)),
  };
}

/** The picture centred in the frame at the given zoom. */
export function centered(imageWidth, imageHeight, zoom = 1) {
  const scale = baseScale(imageWidth, imageHeight) * zoom;
  return clampOffset({ x: (FRAME_WIDTH - imageWidth * scale) / 2, y: (FRAME_HEIGHT - imageHeight * scale) / 2 }, imageWidth, imageHeight, zoom);
}

/** Changes the zoom keeping the same point of the picture at the centre of the frame. */
export function zoomAround(offset, imageWidth, imageHeight, from, to) {
  const base = baseScale(imageWidth, imageHeight);
  const centreX = (FRAME_WIDTH / 2 - offset.x) / (base * from);
  const centreY = (FRAME_HEIGHT / 2 - offset.y) / (base * from);
  return clampOffset({ x: FRAME_WIDTH / 2 - centreX * base * to, y: FRAME_HEIGHT / 2 - centreY * base * to }, imageWidth, imageHeight, to);
}

/** The rectangle of the picture, in its own pixels, that the frame shows. */
export function sourceRect(offset, imageWidth, imageHeight, zoom) {
  const scale = baseScale(imageWidth, imageHeight) * zoom;
  return { x: (0 - offset.x) / scale, y: (0 - offset.y) / scale, width: FRAME_WIDTH / scale, height: FRAME_HEIGHT / scale };
}

/** Cuts the picture to 600×900 and gives the JPEG. A transparent picture goes over white: a JPEG has no transparency. */
export function cutToBlob(image, offset, zoom) {
  const rect = sourceRect(offset, image.naturalWidth, image.naturalHeight, zoom);
  const canvas = document.createElement('canvas');
  canvas.width = OUTPUT_WIDTH;
  canvas.height = OUTPUT_HEIGHT;
  const context = canvas.getContext('2d');
  context.fillStyle = '#ffffff';
  context.fillRect(0, 0, OUTPUT_WIDTH, OUTPUT_HEIGHT);
  context.drawImage(image, rect.x, rect.y, rect.width, rect.height, 0, 0, OUTPUT_WIDTH, OUTPUT_HEIGHT);
  return new Promise((resolve, reject) => {
    canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error('The cut could not be made'))), 'image/jpeg', 0.88);
  });
}
