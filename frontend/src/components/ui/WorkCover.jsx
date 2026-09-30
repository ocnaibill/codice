import { useState } from 'react';
import { authenticatedUrl } from '../../lib/api';
import { LibraryIcon } from './LibraryIcon';

export function WorkCover({ item, className = '' }) {
  const src = item.coverUrl ? authenticatedUrl(item.coverUrl) : null;
  const [failedSrc, setFailedSrc] = useState(null);
  return src && failedSrc !== src ? (
    <img
      src={src}
      alt={item.title}
      loading="lazy"
      className={className}
      onError={() => setFailedSrc(src)}
    />
  ) : (
    <span
      role="img"
      aria-label={item.title}
      className={`work-cover-placeholder ${className}`}
    >
      <LibraryIcon name="book" />
      <span>{item.title}</span>
      <small>{item.author}</small>
    </span>
  );
}
