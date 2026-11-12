import { useRouter } from 'expo-router';
import React, { useEffect, useState } from 'react';
import { ActivityIndicator, Platform, Pressable, ScrollView, Share, Text, View } from 'react-native';
import * as Clipboard from 'expo-clipboard';
import { useEmbeddedEthereumWallet, useLoginWithOAuth, usePrivy, OAuthProviderID } from '@privy-io/expo';
import QRCodeStyled from 'react-native-qrcode-styled';
import { getDb } from '@/db/client';
import { useFocusData } from '@/hooks/useFocusData';
import { useKernelClient } from '@/hooks/use-Kernal';
import { shortenAddress } from '@/lib/wallet';
import { getBusiness } from '@/services/business';
import { toast } from '@/shared/ui/molecules/Toast';
import { useWallet } from '@/store/wallet';
import { colors, radius, space } from '@/theme/colors';
import { Button, Card, Row, SectionTitle } from '@/ui/components';

/**
 * Wallet tab: signs the merchant in with Privy, derives the ZeroDev smart
 * account and shows the address used to receive customer payments.
 */
export default function WalletScreen() {
  const db = getDb();
  const router = useRouter();
  const { isReady, user, logout } = usePrivy();
  const { login } = useLoginWithOAuth();
  const { wallets, create } = useEmbeddedEthereumWallet();
  const wallet = wallets?.[0];
  const { kernelAddress, loading } = useKernelClient(wallet);
  const { setWallet } = useWallet();
  const [business] = useFocusData(() => getBusiness(db));
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (user && wallets.length === 0) {
      create().catch((e: unknown) => toast.error(e instanceof Error ? e.message : 'Could not create wallet'));
    }
  }, [user, wallets.length, create]);

  useEffect(() => {
    if (wallet?.address && kernelAddress) {
      setWallet({ smartContractDeployed: true, address: wallet.address, smartAdress: kernelAddress });
    }
  }, [wallet?.address, kernelAddress, setWallet]);

  const activeAddress = kernelAddress ?? wallet?.address ?? '';

  const signIn = async (provider: OAuthProviderID) => {
    try {
      await login({ provider });
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Sign in failed');
    }
  };

  const copy = async () => {
    await Clipboard.setStringAsync(activeAddress);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <ScrollView contentContainerStyle={{ padding: space.lg, paddingBottom: 60 }} testID="wallet-screen">
      <Text style={{ fontSize: 30, fontWeight: '800', color: colors.ink, letterSpacing: -0.8 }}>Wallet</Text>
      <Text style={{ color: colors.muted, marginTop: 4, marginBottom: space.lg }}>
        Receive customer payments directly into your smart account.
      </Text>

      {!isReady ? (
        <Card style={{ alignItems: 'center', paddingVertical: 32 }}>
          <ActivityIndicator color={colors.ink} />
        </Card>
      ) : !user ? (
        <Card>
          <Text style={{ fontWeight: '800', color: colors.ink, fontSize: 16 }}>Sign in to activate your wallet</Text>
          <Text style={{ color: colors.muted, marginTop: 4, marginBottom: space.md }}>
            Your wallet is created with Privy and secured by ZeroDev. You can sign in with Apple or Google.
          </Text>
          <Button
            testID="wallet-signin"
            variant="lime"
            label={Platform.OS === 'ios' ? 'Sign in with Apple' : 'Sign in with Google'}
            onPress={() => signIn(Platform.OS === 'ios' ? 'apple' : 'google')}
          />
        </Card>
      ) : (
        <>
          <Card style={{ alignItems: 'center' }} testID="wallet-card">
            {loading && !activeAddress ? (
              <ActivityIndicator color={colors.ink} />
            ) : (
              <>
                <QRCodeStyled data={activeAddress} padding={16} size={190} pieceBorderRadius={4} color={colors.ink} />
                <Text testID="wallet-address" selectable style={{ marginTop: space.md, fontWeight: '700', color: colors.ink }}>
                  {shortenAddress(activeAddress, 10, 8)}
                </Text>
                <Text style={{ color: colors.muted, fontSize: 12, marginTop: 2 }}>
                  {kernelAddress ? 'ZeroDev smart account' : 'Embedded wallet'}
                </Text>
                <View style={{ flexDirection: 'row', gap: 8, marginTop: space.md }}>
                  <Button small variant="ghost" testID="wallet-copy" label={copied ? 'Copied' : 'Copy address'} onPress={copy} />
                  <Button
                    small
                    variant="ghost"
                    label="Share"
                    onPress={() => Share.share({ message: activeAddress }).catch(() => undefined)}
                  />
                </View>
              </>
            )}
          </Card>

          <SectionTitle>Business</SectionTitle>
          <Card>
            <Row left="Name" right={business?.name ?? '—'} testID="wallet-business-name" />
            <Row left="Website" right={business?.website || '—'} />
            <Row left="Address" right={business?.address || '—'} />
            <View style={{ height: space.md }} />
            <Button testID="wallet-edit-business" variant="ghost" label="Edit business details" onPress={() => router.push('/onboarding')} />
          </Card>

          <View style={{ height: space.md }} />
          <Button testID="wallet-signout" variant="ghost" label="Sign out" onPress={() => logout().catch(() => undefined)} />
        </>
      )}

      <View style={{ backgroundColor: colors.canvas, borderRadius: radius.md, padding: space.md, marginTop: space.lg }}>
        <Text style={{ color: colors.muted, fontSize: 12 }}>
          Customers pay Bitcoin, USDC, ZARP or Euro on Base or Ethereum. The receipt worker watches the chain and confirms each receipt
          automatically.
        </Text>
      </View>
    </ScrollView>
  );
}
