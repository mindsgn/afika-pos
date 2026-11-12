import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ScrollView, Text, View } from 'react-native';
import { getDb } from '@/db/client';
import { Discount, NO_DISCOUNT } from '@/domain/discount';
import { maxRedeemablePoints, pointsEarned } from '@/domain/loyalty';
import { formatZAR, parseToCents } from '@/domain/money';
import { PAYMENT_OPTIONS, formatAssetAmount, getPaymentOption, toAssetAmount } from '@/domain/payment';
import { createCustomer, listCustomers } from '@/services/customers';
import { startPayment } from '@/services/payments';
import { toast } from '@/shared/ui/molecules/Toast';
import { useCart } from '@/state/cart';
import { colors, radius, space } from '@/theme/colors';
import { Button, Card, Chip, Empty, Field, Row, SectionTitle, Stepper } from '@/ui/components';

export default function CartScreen() {
  const db = getDb();
  const router = useRouter();
  const cart = useCart();
  const [discountMode, setDiscountMode] = useState<'none' | 'percent' | 'fixed'>('none');
  const [discountText, setDiscountText] = useState('');
  const [busy, setBusy] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const [custQuery, setCustQuery] = useState('');
  const [newName, setNewName] = useState('');
  const [newPhone, setNewPhone] = useState('');

  const applyDiscount = (mode: 'none' | 'percent' | 'fixed', text: string) => {
    setDiscountMode(mode);
    setDiscountText(text);
    let d: Discount = NO_DISCOUNT;
    if (mode === 'percent') {
      const n = Number(text.replace(',', '.'));
      if (Number.isFinite(n) && n > 0) d = { type: 'percent', value: Math.min(100, n) };
    } else if (mode === 'fixed') {
      const c = parseToCents(text);
      if (c && c > 0) d = { type: 'fixed', value: c };
    }
    cart.setDiscount(d);
  };

  const { totals } = cart;
  const option = getPaymentOption(cart.method);
  const maxPoints = cart.customer
    ? maxRedeemablePoints(cart.customer.loyaltyPoints, totals.subtotal - totals.discountAmount)
    : 0;

  const pay = async () => {
    setBusy(true);
    try {
      const started = await startPayment(db, {
        items: cart.lines.map((l) => ({ productId: l.productId, qty: l.qty })),
        discount: cart.discount,
        customerId: cart.customer?.id ?? null,
        redeemPoints: cart.redeemPoints,
        method: cart.method,
        optionId: cart.method,
      });
      router.push(`/checkout/${started.payment.id}`);
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not start checkout');
    } finally {
      setBusy(false);
    }
  };

  if (cart.lines.length === 0) {
    return <Empty text="Your cart is empty" />;
  }

  const customers = pickerOpen ? listCustomers(db, custQuery) : [];

  const setQtyWithToast = (productId: number, qty: number) => {
    const err = cart.setQty(productId, qty);
    if (err) toast.error(err);
  };

  return (
    <ScrollView contentContainerStyle={{ padding: space.lg, paddingBottom: 60 }} keyboardShouldPersistTaps="handled">
      <Card>
        {cart.lines.map((l) => (
          <View key={l.productId} style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingVertical: 8 }}>
            <View style={{ flex: 1 }}>
              <Text style={{ fontWeight: '700', color: colors.ink }}>{l.name}</Text>
              <Text style={{ color: colors.muted, fontSize: 12 }}>{formatZAR(l.unitPrice)} each</Text>
            </View>
            <Stepper value={l.qty} max={l.stock} testID={`line-${l.sku}`} onChange={(q) => setQtyWithToast(l.productId, q)} />
            <Text style={{ width: 80, textAlign: 'right', fontWeight: '700' }}>{formatZAR(l.unitPrice * l.qty)}</Text>
          </View>
        ))}
      </Card>

      <SectionTitle>Discount</SectionTitle>
      <Card>
        <View style={{ flexDirection: 'row', marginBottom: space.md }}>
          <Chip label="None" active={discountMode === 'none'} onPress={() => applyDiscount('none', '')} testID="discount-none" />
          <Chip label="% off" active={discountMode === 'percent'} onPress={() => applyDiscount('percent', discountText)} testID="discount-percent" />
          <Chip label="R off" active={discountMode === 'fixed'} onPress={() => applyDiscount('fixed', discountText)} testID="discount-fixed" />
        </View>
        {discountMode !== 'none' && (
          <Field
            testID="discount-value"
            value={discountText}
            onChangeText={(t) => applyDiscount(discountMode, t)}
            keyboardType="decimal-pad"
            placeholder={discountMode === 'percent' ? 'e.g. 10' : 'e.g. 15.00'}
            style={{ marginBottom: 0 }}
          />
        )}
      </Card>

      <SectionTitle>Customer & loyalty</SectionTitle>
      <Card>
        {cart.customer ? (
          <View>
            <Row left="Customer" right={cart.customer.name} />
            <Row left="Points balance" right={String(cart.customer.loyaltyPoints)} testID="points-balance" />
            <View style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', marginTop: 6 }}>
              <Text style={{ color: colors.muted }}>Redeem points (10c each)</Text>
              <Stepper value={cart.redeemPoints} min={0} max={maxPoints} testID="redeem" onChange={cart.setRedeemPoints} />
            </View>
            <View style={{ flexDirection: 'row', marginTop: space.md, gap: 8 }}>
              <Button small variant="ghost" label="Use max" testID="redeem-max" onPress={() => cart.setRedeemPoints(maxPoints)} />
              <Button small variant="ghost" label="Remove customer" testID="customer-remove" onPress={() => cart.setCustomer(null)} />
            </View>
          </View>
        ) : (
          <Button small variant="ghost" label={pickerOpen ? 'Close' : 'Add customer'} testID="customer-toggle" onPress={() => setPickerOpen(!pickerOpen)} />
        )}
        {pickerOpen && !cart.customer && (
          <View style={{ marginTop: space.md }}>
            <Field testID="customer-search" value={custQuery} onChangeText={setCustQuery} placeholder="Search name or phone" autoCapitalize="none" />
            {customers.map((c) => (
              <View key={c.id} style={{ marginBottom: 6 }}>
                <Button
                  small
                  variant="ghost"
                  testID={`customer-row-${c.id}`}
                  label={`${c.name} · ${c.loyaltyPoints} pts`}
                  onPress={() => {
                    cart.setCustomer(c);
                    setPickerOpen(false);
                  }}
                />
              </View>
            ))}
            <Text style={{ color: colors.muted, marginVertical: 6, fontSize: 12 }}>Or create a new customer</Text>
            <Field testID="customer-name-input" value={newName} onChangeText={setNewName} placeholder="Name" />
            <Field testID="customer-phone-input" value={newPhone} onChangeText={setNewPhone} placeholder="Phone (optional)" keyboardType="phone-pad" />
            <Button
              small
              testID="customer-add"
              label="Save & select"
              onPress={() => {
                try {
                  const c = createCustomer(db, { name: newName, phone: newPhone });
                  cart.setCustomer(c);
                  setPickerOpen(false);
                  setNewName('');
                  setNewPhone('');
                } catch (e) {
                  toast.error(e instanceof Error ? e.message : 'Could not save customer');
                }
              }}
            />
          </View>
        )}
      </Card>

      <SectionTitle>Payment</SectionTitle>
      <Card>
        <Text style={{ color: colors.muted, fontSize: 12, marginBottom: space.sm }}>
          The customer chooses an asset on the secure checkout page. Bitcoin or a stablecoin on Base or Ethereum.
        </Text>
        <View style={{ flexDirection: 'row', flexWrap: 'wrap' }}>
          {PAYMENT_OPTIONS.map((m) => (
            <Chip key={m.id} label={m.label} active={cart.method === m.id} onPress={() => cart.setMethod(m.id)} testID={`method-${m.id}`} />
          ))}
        </View>
        <View style={{ marginTop: space.md, backgroundColor: colors.canvas, borderRadius: radius.md, padding: space.md }}>
          <Text style={{ color: colors.muted, fontSize: 12 }}>Customer pays about</Text>
          <Text testID="crypto-amount" style={{ marginTop: 2, color: colors.ink, fontWeight: '800', fontSize: 20 }}>
            {formatAssetAmount(toAssetAmount(totals.total, option.id), option.id)}
          </Text>
          <Text style={{ color: colors.muted, fontSize: 12, marginTop: 2 }}>Indicative rate · final amount is locked on the checkout link</Text>
        </View>
      </Card>

      <Card style={{ backgroundColor: colors.ink }}>
        <Row color={colors.muted} left="Subtotal" right={formatZAR(totals.subtotal)} testID="total-subtotal" />
        {totals.discountAmount > 0 && <Row color={colors.muted} left="Discount" right={`-${formatZAR(totals.discountAmount)}`} testID="total-discount" />}
        {totals.pointsDiscount > 0 && <Row color={colors.muted} left="Points" right={`-${formatZAR(totals.pointsDiscount)}`} testID="total-points" />}
        <Row color={colors.lime} bold left="Total" right={formatZAR(totals.total)} testID="total-due" />
        <Text style={{ color: colors.muted, fontSize: 12 }}>Earns {pointsEarned(totals.total)} loyalty points</Text>
      </Card>

      <Button
        testID="pay-button"
        variant="lime"
        disabled={busy}
        label={busy ? 'Creating payment link…' : `Charge ${formatZAR(totals.total)}`}
        onPress={pay}
      />
      <View style={{ height: 8 }} />
      <Button testID="clear-cart" variant="ghost" label="Clear cart" onPress={() => { cart.clear(); setDiscountMode('none'); setDiscountText(''); router.back(); }} />
    </ScrollView>
  );
}
