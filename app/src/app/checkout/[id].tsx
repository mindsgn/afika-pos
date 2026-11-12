import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { ActivityIndicator, Linking, Pressable, ScrollView, Share, Text, View } from 'react-native';
import QRCodeStyled from 'react-native-qrcode-styled';
import { getDb } from '@/db/client';
import { formatZAR } from '@/domain/money';
import { formatAssetAmount } from '@/domain/payment';
import { useFocusData } from '@/hooks/useFocusData';
import { abandonPayment, getPayment, refreshPayment, simulatePayment } from '@/services/payments';
import { toast } from '@/shared/ui/molecules/Toast';
import { useCart } from '@/state/cart';
import { colors, radius, space } from '@/theme/colors';
import { Button, Card, SectionTitle } from '@/ui/components';

const POLL_MS = 3000;

export default function CheckoutScreen() {
  const db = getDb();
  const router = useRouter();
  const cart = useCart();
  const { id } = useLocalSearchParams<{ id: string }>();
  const [payment, refreshLocal] = useFocusData(() => getPayment(db, id), [id]);
  const [checking, setChecking] = useState(false);
  const settled = useRef(false);

  const settle = useCallback(() => {
    if (!payment || settled.current) return;
    if (payment.status !== 'paid') return;
    settled.current = true;
    cart.clear();
    router.replace(`/sale/${payment.saleId}`);
  }, [payment, cart, router]);

  const check = useCallback(async () => {
    if (!payment || payment.status !== 'pending') return;
    setChecking(true);
    try {
      const next = await refreshPayment(db, payment.id);
      refreshLocal();
      if (next.status === 'failed') toast.error('This payment is no longer available.');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not check payment status');
    } finally {
      setChecking(false);
    }
  }, [db, payment, refreshLocal]);

  // Poll while pending so the screen completes as soon as the worker sees the tx.
  useEffect(() => {
    if (!payment || payment.status !== 'pending') return;
    const timer = setInterval(check, POLL_MS);
    return () => clearInterval(timer);
  }, [check, payment]);

  useEffect(() => {
    settle();
  }, [settle]);

  if (!payment) {
    return (
      <View style={{ flex: 1, alignItems: 'center', justifyContent: 'center', padding: space.lg }}>
        <Text style={{ color: colors.muted }}>Payment request not found.</Text>
      </View>
    );
  }

  const paid = payment.status === 'paid';
  const pending = payment.status === 'pending';

  const share = async () => {
    await Share.share({
      message: `Pay ${formatZAR(payment.amountCents)} to complete order ${payment.receiptNo}: ${payment.paymentUrl}`,
      url: payment.paymentUrl,
    });
  };

  const simulate = async () => {
    setChecking(true);
    try {
      await simulatePayment(db, payment.id);
      refreshLocal();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not confirm payment');
    } finally {
      setChecking(false);
    }
  };

  const cancel = () => {
    try {
      abandonPayment(db, payment.id);
    } catch {
      // already settled
    }
    cart.clear();
    router.replace('/(tabs)');
  };

  return (
    <ScrollView contentContainerStyle={{ padding: space.lg, paddingBottom: 60 }} testID="checkout-screen">
      <View style={{ alignItems: 'center', marginBottom: space.lg }}>
        <Text style={{ color: colors.muted, fontSize: 12, fontWeight: '700', letterSpacing: 1 }}>RECEIPT {payment.receiptNo}</Text>
        <Text testID="checkout-amount" style={{ color: colors.ink, fontSize: 36, fontWeight: '800', marginTop: 6 }}>
          {formatZAR(payment.amountCents)}
        </Text>
        <Text testID="checkout-asset" style={{ color: colors.muted, marginTop: 2 }}>
          {formatAssetAmount(Number(payment.cryptoAmount), payment.asset)}
        </Text>
      </View>

      <Card style={{ alignItems: 'center' }}>
        <View style={{ backgroundColor: colors.paper, borderRadius: 12, padding: 4 }}>
          <QRCodeStyled
            data={payment.paymentUrl}
            padding={18}
            size={220}
            pieceBorderRadius={4}
            color={colors.ink}
          />
        </View>
        <Text style={{ color: colors.muted, fontSize: 12, marginTop: space.md, textAlign: 'center' }}>
          Scan to open the checkout page
        </Text>
        <Text testID="checkout-link" selectable style={{ color: colors.ink, fontSize: 12, marginTop: 6, textAlign: 'center' }} numberOfLines={2}>
          {payment.paymentUrl}
        </Text>
      </Card>

      <View style={{ flexDirection: 'row', gap: 8, marginTop: space.md }}>
        <Button style={{ flex: 1 }} testID="checkout-share" variant="primary" label="Share link" onPress={share} />
        <Button
          style={{ flex: 1 }}
          testID="checkout-open"
          variant="ghost"
          label="Open in browser"
          onPress={() => Linking.openURL(payment.paymentUrl).catch(() => toast.error('Could not open the checkout page'))}
        />
      </View>

      <SectionTitle>Status</SectionTitle>
      <Card testID="checkout-status">
        <View style={{ flexDirection: 'row', alignItems: 'center', gap: 10 }}>
          {pending ? <ActivityIndicator color={colors.ink} /> : null}
          <Text style={{ fontWeight: '800', color: paid ? colors.green : pending ? colors.ink : colors.red, fontSize: 16 }}>
            {paid ? 'Payment received' : pending ? 'Waiting for payment…' : payment.status === 'expired' ? 'Link expired' : 'Payment failed'}
          </Text>
        </View>
        {pending && (
          <View style={{ marginTop: space.md }}>
            <Text style={{ color: colors.muted, fontSize: 12 }}>
              Send {formatAssetAmount(Number(payment.cryptoAmount), payment.asset)} on {payment.network}. This screen updates
              automatically when the transaction is confirmed.
            </Text>
            {payment.address ? (
              <Text selectable style={{ color: colors.ink, fontSize: 12, marginTop: 8 }}>{payment.address}</Text>
            ) : null}
          </View>
        )}
      </Card>

      {pending && (
        <>
          <Button testID="checkout-check" variant="lime" disabled={checking} label={checking ? 'Checking…' : "I've paid — check now"} onPress={check} />
          <View style={{ height: 8 }} />
          <Button testID="checkout-simulate" variant="ghost" disabled={checking} label="Simulate payment (demo)" onPress={simulate} />
          <View style={{ height: 8 }} />
          <Button testID="checkout-cancel" variant="ghost" label="Cancel payment" onPress={cancel} />
        </>
      )}

      {!pending && (
        <Pressable testID="checkout-done" onPress={() => router.replace('/(tabs)')} style={{ marginTop: space.md, alignItems: 'center' }}>
          <Text style={{ color: colors.muted, fontWeight: '700' }}>Back to sell</Text>
        </Pressable>
      )}

      <View style={{ backgroundColor: colors.canvas, borderRadius: radius.md, padding: space.md, marginTop: space.lg }}>
        <Text style={{ color: colors.muted, fontSize: 12 }}>
          The link is shareable. The customer can open it in a browser or in the Afika app and pick Bitcoin, USDC, ZARP or Euro on
          Base or Ethereum.
        </Text>
      </View>
    </ScrollView>
  );
}
