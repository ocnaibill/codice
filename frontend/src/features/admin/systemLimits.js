const HOUR = 60 * 60 * 1000;

// A backup a day is the goal (DEC-064): past a day and a half the last one is called out.
export const STALE_AFTER = 36 * HOUR;
// A job that has been due this long and has not started: the worker may be stopped.
export const STUCK_AFTER = 15 * 60 * 1000;
// Below this share of free space the storage is called out.
export const LOW_SPACE = 0.15;
