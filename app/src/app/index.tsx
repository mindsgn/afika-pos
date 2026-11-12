import { useRouter } from 'expo-router';
import React, { useEffect } from 'react';
import { usePrivy } from '@privy-io/expo';
import { ActivityIndicator, View } from 'react-native';
import { getDb } from '@/db/client';
import { E2E_ENABLED, isE2eSignedIn } from '@/lib/e2e';
import { hasBusiness } from '@/services/business';
import { colors } from '@/theme/colors';

/**
 * Boot screen: sends first-run merchants to sign-in and onboarding, and
 * everyone else to the sell tab.
 */
export default function BootScreen() {
  const router = useRouter();
  const { isReady, user } = usePrivy();

  useEffect(() => {
    // The E2E bypass does not wait for Privy to finish initialising.
    if (!isReady && !E2E_ENABLED) return;
    const setupComplete = hasBusiness(getDb());
    if (setupComplete) router.replace('/(tabs)');
    else if (user || isE2eSignedIn()) router.replace('/onboarding');
    else router.replace('/welcome');
  }, [router, isReady, user]);

  return (
    <View testID="boot-screen" style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: colors.ink }}>
      <ActivityIndicator color={colors.lime} />
    </View>
  );
}
