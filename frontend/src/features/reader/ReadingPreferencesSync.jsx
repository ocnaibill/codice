import { useEffect, useRef } from 'react';
import { usePreferences } from '../auth/api/usePreferences';
import { sanitizeSettings } from './epubThemes';
import { getEpubSettings, hasSavedEpubSettings, saveEpubSettings, setPreferenceOwner } from './preferences';
import { pushReadingSettings } from './readingSync';

/**
 * Brings how the text looks, from the server to this device, once for each account that signs in (#106): what the person
 * chose on another device is what this one starts with. If the server has none, what was chosen on this device is sent to it,
 * so that a choice made before this existed is not lost. It draws nothing.
 */
export function ReadingPreferencesSync({ userId, enabled = true }) {
  const { data } = usePreferences(enabled && !!userId);
  const adopted = useRef(false);
  useEffect(() => {
    if (!data || !userId || adopted.current) return;
    adopted.current = true;
    setPreferenceOwner(userId); // the choice is kept for this account, whatever the order the effects of the page ran in
    if (data.reader && typeof data.reader === 'object') {
      saveEpubSettings(sanitizeSettings(data.reader));
    } else if (hasSavedEpubSettings()) {
      pushReadingSettings(getEpubSettings(), 0);
    }
  }, [data, userId]);
  return null;
}
