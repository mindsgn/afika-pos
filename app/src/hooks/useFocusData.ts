import { useFocusEffect } from 'expo-router';
import { useCallback, useRef, useState } from 'react';

/** Loads synchronous data (local SQLite) on mount, on focus and when `deps` change. */
export function useFocusData<T>(load: () => T, deps: unknown[] = []) {
  const loadRef = useRef(load);
  loadRef.current = load;
  const [data, setData] = useState<T>(() => load());
  const refresh = useCallback(() => setData(loadRef.current()), []);
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useFocusEffect(useCallback(() => refresh(), [refresh, ...deps]));
  return [data, refresh] as const;
}
