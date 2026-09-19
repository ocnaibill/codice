import iconStarFull from '../../assets/icons/card-star-full.svg';
import iconStarHalf from '../../assets/icons/card-star-half.svg';

export function StarRating({ rating = 5, outOf = 5 }) {
  const full = Math.floor(rating);
  const hasHalf = rating - full >= 0.5;
  const empty = outOf - full - (hasHalf ? 1 : 0);

  return (
    <div className="flex items-center gap-0.5">
      {Array.from({ length: full }).map((_, i) => (
        <img key={`f${i}`} src={iconStarFull} alt="" className="size-[11px]" />
      ))}
      {hasHalf && <img src={iconStarHalf} alt="" className="size-[11px]" />}
      {Array.from({ length: Math.max(empty, 0) }).map((_, i) => (
        <img key={`e${i}`} src={iconStarHalf} alt="" className="size-[11px] opacity-20" />
      ))}
    </div>
  );
}
