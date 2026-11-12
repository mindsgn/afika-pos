import { useRouter } from 'expo-router';
import React, { useEffect } from 'react';
import { usePrivy } from '@privy-io/expo';
import { ActivityIndicator, View } from 'react-native';
import { getDb } from '@/db/client';
import { hasBusiness } from '@/services/business';
import { colors } from '@/theme/colors';

/**
 * Boot screen: sends first-run merchants to onboarding and everyone else to
 * the sell tab.
 */
export default function BootScreen() {
  const router = useRouter();
  const { isReady, user } = usePrivy();

  useEffect(() => {
    if (!isReady) return;
    const setupComplete = hasBusiness(getDb());
    router.replace(setupComplete ? '/(tabs)' : user ? '/onboarding' : '/welcome');
  }, [router, isReady, user]);

  return (
    <View testID="boot-screen" style={{ flex: 1, alignItems: 'center', justifyContent: 'center', backgroundColor: colors.ink }}>
      <ActivityIndicator color={colors.lime} />
    </View>
  );
}
