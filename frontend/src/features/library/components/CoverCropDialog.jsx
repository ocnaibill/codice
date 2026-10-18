import React, { useEffect, useRef, useState } from 'react';
import { useDialog } from '../../../lib/useDialog';
import {
  BANNER, COVER, MAX_ZOOM, baseScale, centered, clampOffset, cutToBlob, zoomAround,
} from '../coverCrop';

const STEP = 12;

/**
 * Where a person cuts a picture to the proportion every cover has (2:3): the picture moves under a frame that does not, and the
 * zoom is a slider. What leaves here is the cut (a JPEG of 600×900), which is what the server is sent, and the frame is what the
 * card will show.
 *
 * `file` is the picture chosen; onConfirm(blob) is called with the cut; `busy` and `error` are of whoever sends it. With `banner` the
 * frame is the wide one of the banner of a series (3:1, DEC-201).
 */
export function CoverCropDialog({ file, onConfirm, onCancel, busy = false, error = '', banner = false }) {
  const shape = banner ? BANNER : COVER;
  const dialogRef = useRef(null);
  const frameRef = useRef(null);
  // The focus goes to the frame, so that the arrow keys move the picture at once (and the dialog is not scrolled to a button).
  useDialog(dialogRef, { onEscape: busy ? undefined : onCancel, initialFocus: frameRef });

  const [image, setImage] = useState(null);
  const [failed, setFailed] = useState(false);
  const [zoom, setZoom] = useState(1);
  const [offset, setOffset] = useState({ x: 0, y: 0 });
  const [cutting, setCutting] = useState(false);
  const [cutFailed, setCutFailed] = useState(false);
  const drag = useRef(null);

  useEffect(() => {
    const url = URL.createObjectURL(file);
    const picture = new Image();
    picture.onload = () => {
      setImage(picture);
      setZoom(1);
      setOffset(centered(picture.naturalWidth, picture.naturalHeight, 1, shape));
    };
    picture.onerror = () => setFailed(true);
    picture.src = url;
    return () => URL.revokeObjectURL(url);
  }, [file, shape]);

  const width = image?.naturalWidth;
  const height = image?.naturalHeight;
  const scale = image ? baseScale(width, height, shape) * zoom : 1;

  const move = (dx, dy) => image && setOffset((current) => clampOffset({ x: current.x + dx, y: current.y + dy }, width, height, zoom, shape));
  const changeZoom = (next) => {
    if (!image) return;
    setOffset((current) => zoomAround(current, width, height, zoom, next, shape));
    setZoom(next);
  };

  const onPointerDown = (event) => {
    if (!image) return;
    event.currentTarget.setPointerCapture?.(event.pointerId);
    drag.current = { x: event.clientX, y: event.clientY };
  };
  const onPointerMove = (event) => {
    if (!drag.current) return;
    move(event.clientX - drag.current.x, event.clientY - drag.current.y);
    drag.current = { x: event.clientX, y: event.clientY };
  };
  const onPointerUp = () => { drag.current = null; };
  const onKeyDown = (event) => {
    const arrows = { ArrowLeft: [STEP, 0], ArrowRight: [-STEP, 0], ArrowUp: [0, STEP], ArrowDown: [0, -STEP] };
    if (!arrows[event.key]) return;
    event.preventDefault();
    move(...arrows[event.key]);
  };

  const confirm = async () => {
    setCutFailed(false);
    setCutting(true);
    try {
      onConfirm(await cutToBlob(image, offset, zoom, shape));
    } catch {
      setCutFailed(true);
    } finally {
      setCutting(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-[60] flex animate-fade-in items-center justify-center bg-black/50 p-4"
      onClick={(event) => event.target === event.currentTarget && !busy && onCancel()}
    >
      <div ref={dialogRef} role="dialog" aria-modal="true" aria-label={banner ? 'Recortar o fundo' : 'Recortar a capa'} className={`max-h-full w-full ${banner ? 'max-w-md' : 'max-w-sm'} animate-pop-in overflow-y-auto rounded-xl bg-white p-5 shadow-2xl`}>
        <h2 className="font-display text-xl font-semibold text-ink">{banner ? 'Recortar o fundo' : 'Recortar a capa'}</h2>
        <p className="mt-2 text-[13px] leading-relaxed text-ink-soft">
          {banner
            ? 'Arraste a imagem e ajuste o zoom até o fundo ficar como você quer. O fundo da série tem a proporção 3:1, larga como uma faixa.'
            : 'Arraste a imagem e ajuste o zoom até a capa ficar como você quer. Toda capa do acervo tem a proporção 2:3.'}
        </p>

        {failed ? (
          <p role="alert" className="mt-4 text-sm text-danger">Não foi possível abrir essa imagem. Escolha um JPEG, PNG ou WebP.</p>
        ) : (
          <>
            <div
              ref={frameRef}
              role="group"
              tabIndex={0}
              aria-label={`Área d${banner ? 'o fundo' : 'a capa'}. Use as setas do teclado para mover a imagem.`}
              onPointerDown={onPointerDown}
              onPointerMove={onPointerMove}
              onPointerUp={onPointerUp}
              onPointerCancel={onPointerUp}
              onKeyDown={onKeyDown}
              data-testid="cover-frame"
              className="relative mx-auto mt-4 cursor-grab touch-none select-none overflow-hidden rounded-md bg-surface-alt outline-none ring-1 ring-border-hairline focus-visible:ring-2 focus-visible:ring-brand active:cursor-grabbing"
              style={{ width: shape.frameWidth, height: shape.frameHeight }}
            >
              {image && (
                <img
                  src={image.src}
                  alt=""
                  draggable={false}
                  className="pointer-events-none absolute left-0 top-0 max-w-none"
                  style={{ width: width * scale, height: height * scale, transform: `translate(${offset.x}px, ${offset.y}px)` }}
                />
              )}
            </div>
            <label className="mt-4 flex items-center gap-3 text-xs font-medium text-ink-soft">
              Zoom
              <input
                type="range"
                min="1"
                max={MAX_ZOOM}
                step="0.01"
                value={zoom}
                onChange={(event) => changeZoom(parseFloat(event.target.value))}
                disabled={!image || busy}
                className="min-w-0 flex-1"
              />
            </label>
          </>
        )}

        {(error || cutFailed) && (
          <p role="alert" className="mt-3 text-sm text-danger">{error || 'Não foi possível recortar a imagem.'}</p>
        )}

        <div className="mt-6 flex flex-wrap justify-end gap-2">
          <button type="button" onClick={onCancel} disabled={busy} className="min-h-10 rounded px-4 py-2 text-[13px] text-ink-soft hover:text-ink disabled:opacity-40">
            Cancelar
          </button>
          <button
            type="button"
            onClick={confirm}
            disabled={!image || busy || cutting}
            className="min-h-10 rounded bg-brand px-4 py-2 text-[13px] font-medium text-white hover:brightness-110 disabled:cursor-not-allowed disabled:opacity-40"
          >
            {busy || cutting ? 'Salvando…' : `Usar ${banner ? 'este fundo' : 'esta capa'}`}
          </button>
        </div>
      </div>
    </div>
  );
}
