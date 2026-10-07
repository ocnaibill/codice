import { useEffect, useRef } from 'react';
import { usePreferences } from '../auth/api/usePreferences';
import { sanitizeSettings } from './epubThemes';
import { getEpubSettings, hasSavedEpubSettings, saveEpubSettings, setPreferenceOwner } from './preferences';
import { pushReadingSettings } from './readingSync';
import { deviceClass, otherDevice } from './deviceClass';

/**
 * Brings how the text looks, from the server to this device, once for each account that signs in (#106): what the person
 * chose for this kind of device (a phone, a computer) is what this one starts with. A kind that has no choice yet starts from
 * the other kind's, once, and that copy is its own from then on (#180). If the server has none, what was chosen on this device
 * is sent to it, so that a choice made before this existed is not lost. It draws nothing.
 */
export function ReadingPreferencesSync({ userId, enabled = true }) {
  const { data } = usePreferences(enabled && !!userId);
  const adopted = useRef(false);
  useEffect(() => {
    if (!data || !userId || adopted.current) return;
    adopted.current = true;
    setPreferenceOwner(userId); // the choice is kept for this account, whatever the order the effects of the page ran in
    const mine = data.reader?.[deviceClass()];
    const other = data.reader?.[otherDevice(deviceClass())];
    if (mine && typeof mine === 'object') {
      saveEpubSettings(sanitizeSettings(mine));
    } else if (other && typeof other === 'object') {
      const adopted = sanitizeSettings(other);
      saveEpubSettings(adopted);
      pushReadingSettings(adopted, 0); // the first time: the copy is this kind's own choice from now on
    } else if (hasSavedEpubSettings()) {
      pushReadingSettings(getEpubSettings(), 0);
    }
  }, [data, userId]);
  return null;
}
