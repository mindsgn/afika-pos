import { useEffect, useRef } from "react";
import { usePathname } from "expo-router";
import { getAnalytics, logEvent } from '@react-native-firebase/analytics';

export function AnalyticsTracker() {
  const pathname = usePathname();
  const previousPathname = useRef<string | null>(null);

  useEffect(() => {
    const logScreen = async () => {
      if (
        pathname &&
        pathname !== previousPathname.current
      ) {
        await logEvent(getAnalytics(), 'screen_view', {
          screen_name: pathname,
          screen_class: pathname,
        });
        previousPathname.current = pathname;
      }
    };

    logScreen();
  }, [pathname]);

  return null;
}