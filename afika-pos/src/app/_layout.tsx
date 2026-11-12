import Constants from 'expo-constants';
import { Stack } from 'expo-router';
import { PrivyProvider } from '@privy-io/expo';
import { PrivyElements } from '@privy-io/expo/ui';
import { AnalyticsTracker } from '@/components/analytics';
import { getDb } from '@/db/client';
import { Suspense, useMemo } from 'react';
import { ActivityIndicator, View } from 'react-native';
import {
  useFonts,
  Outfit_400Regular,
  Outfit_500Medium,
  Outfit_600SemiBold,
} from '@expo-google-fonts/outfit';
import { colors } from '@/theme';
import { CartProvider } from '@/state/cart';
import { ToastProvider } from '@/shared/ui/molecules/Toast';
import React from 'react';
import '@/lib/i18n';

export default function RootLayout() {
  // Opens (and bootstraps) the local SQLite database before any screen reads it.
  useMemo(() => getDb(), []);

  const [fontsLoaded] = useFonts({
    Outfit_400Regular,
    Outfit_500Medium,
    Outfit_600SemiBold,
  });

  if (!fontsLoaded) {
    return (
      <View style={{ flex: 1, backgroundColor: colors.paper, alignItems: 'center', justifyContent: 'center' }}>
        <ActivityIndicator color={colors.ink} />
      </View>
    );
  }

  return (
    <Suspense
      fallback={
        <View style={{ flex: 1, backgroundColor: colors.paper }}>
          <ActivityIndicator color={colors.ink} />
        </View>
      }
    >
      <PrivyProvider
        appId={Constants.expoConfig?.extra?.privyAppId}
        clientId={Constants.expoConfig?.extra?.privyClientId}
      >
        <AnalyticsTracker />
        <CartProvider>
          <ToastProvider position="top">
            <Stack>
              <Stack.Screen name="index" options={{ headerShown: false }} />
              <Stack.Screen name="welcome" options={{ headerShown: false }} />
              <Stack.Screen name="onboarding" options={{ headerShown: false }} />
              <Stack.Screen name="(tabs)" options={{ headerShown: false }} />
              <Stack.Screen name="cart" options={{ title: 'Cart', headerBackTitle: 'Back' }} />
              <Stack.Screen name="checkout/[id]" options={{ title: 'Payment', headerBackVisible: false }} />
              <Stack.Screen name="product/[id]" options={{ title: 'Product', headerBackTitle: 'Back' }} />
              <Stack.Screen name="categories" options={{ title: 'Categories', headerBackTitle: 'Back' }} />
              <Stack.Screen name="sale/[id]" options={{ title: 'Receipt', headerBackTitle: 'Back' }} />
            </Stack>
          </ToastProvider>
        </CartProvider>
        <PrivyElements />
      </PrivyProvider>
    </Suspense>
  );
}
