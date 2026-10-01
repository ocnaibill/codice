import { describe, it, expect, beforeEach } from 'vitest';
import { useGlobalStore } from './useGlobalStore';

beforeEach(() => useGlobalStore.setState({ metadataWorkId: null, metadataTab: 'suggestions' }));

describe('the metadata of a work (#70)', () => {
  it('opens at the suggestions unless asked for the other part, and closes', () => {
    useGlobalStore.getState().openMetadata(7);
    expect(useGlobalStore.getState()).toMatchObject({ metadataWorkId: 7, metadataTab: 'suggestions' });
    useGlobalStore.getState().openMetadata(8, 'edit');
    expect(useGlobalStore.getState()).toMatchObject({ metadataWorkId: 8, metadataTab: 'edit' });
    useGlobalStore.getState().closeMetadata();
    expect(useGlobalStore.getState().metadataWorkId).toBeNull();
  });
});
