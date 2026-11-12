import { Ionicons } from '@expo/vector-icons';
import { Tabs } from 'expo-router';
import React from 'react';
import type { ColorValue } from 'react-native';
import { colors } from '@/theme/colors';

type IconName = React.ComponentProps<typeof Ionicons>['name'];

const tab = (title: string, id: string, icon: IconName) => ({
  title,
  tabBarButtonTestID: `tab-${id}`,
  tabBarIcon: ({ color, size }: { color: ColorValue; size: number }) => <Ionicons name={icon} size={size} color={color} />,
});

export default function TabsLayout() {
  return (
    <Tabs
      screenOptions={{
        headerStyle: { backgroundColor: colors.canvas },
        headerShadowVisible: false,
        headerTitleStyle: { color: colors.ink, fontWeight: '800' },
        tabBarActiveTintColor: colors.ink,
        tabBarInactiveTintColor: colors.muted,
        tabBarStyle: { backgroundColor: colors.paper, borderTopColor: colors.line },
      }}
    >
      <Tabs.Screen name="index" options={tab('Sell', 'sell', 'cart-outline')} />
      <Tabs.Screen name="products" options={tab('Products', 'products', 'pricetags-outline')} />
      <Tabs.Screen name="stock" options={tab('Stock', 'stock', 'cube-outline')} />
      <Tabs.Screen name="history" options={tab('History', 'history', 'receipt-outline')} />
      <Tabs.Screen name="insights" options={tab('Insights', 'insights', 'stats-chart-outline')} />
      <Tabs.Screen name="wallet" options={tab('Wallet', 'wallet', 'wallet-outline')} />
    </Tabs>
  );
}
