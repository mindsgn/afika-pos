import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { ScrollView, Share, Switch, Text, View } from 'react-native';
import { getDb } from '@/db/client';
import { formatZAR } from '@/domain/money';
import { useFocusData } from '@/hooks/useFocusData';
import { getSale, receiptText, refundReceiptText, refundSale, voidSale } from '@/services/sales';
import { toast } from '@/shared/ui/molecules/Toast';
import { colors, radius, space } from '@/theme/colors';
import { Button, Card, Empty, Field, SectionTitle, StatusBadge, Stepper } from '@/ui/components';

export default function SaleScreen() {
  const db = getDb();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const saleId = Number(id);
  const [detail, refresh] = useFocusData(() => getSale(db, saleId), [saleId]);
  const [mode, setMode] = useState<'none' | 'refund' | 'void'>('none');
  const [qtys, setQtys] = useState<Record<number, number>>({});
  const [restock, setRestock] = useState(true);
  const [reason, setReason] = useState('');
  const [notice, setNotice] = useState<string | null>(null);

  if (!detail) return <Empty text="Sale not found" />;
  const { sale, items } = detail;
  const canVoid = sale.status === 'completed';
  const canRefund = sale.status === 'completed' || sale.status === 'partially_refunded';
  const refundTotalPreview = items.reduce((sum, i) => {
    const q = qtys[i.id] ?? 0;
    if (!q) return sum;
    const remaining = i.qty - i.refundedQty;
    const lineTotal = i.unitPrice * i.qty - i.lineDiscount;
    return sum + (q === remaining ? lineTotal - i.refundedAmount : Math.floor((lineTotal * q) / i.qty));
  }, 0);

  const close = () => {
    setMode('none');
    setQtys({});
    setReason('');
    setRestock(true);
  };

  const doVoid = () => {
    try {
      voidSale(db, saleId, reason);
      close();
      setNotice('Sale voided and stock restored');
      refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not void');
    }
  };

  const doRefund = () => {
    try {
      const res = refundSale(db, {
        saleId,
        items: Object.entries(qtys).map(([saleItemId, qty]) => ({ saleItemId: Number(saleItemId), qty })),
        reason,
        restock,
      });
      close();
      setNotice(`Refunded ${formatZAR(res.amount)}`);
      refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not refund');
    }
  };

  return (
    <ScrollView contentContainerStyle={{ padding: space.lg, paddingBottom: 60 }} keyboardShouldPersistTaps="handled">
      <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: space.md }}>
        <Text testID="sale-receipt-no" style={{ fontSize: 22, fontWeight: '800', color: colors.ink }}>{sale.receiptNo}</Text>
        <View testID="sale-status"><StatusBadge status={sale.status} /></View>
      </View>
      {notice && (
        <View testID="sale-notice" style={{ backgroundColor: '#E3F3EB', borderRadius: radius.md, padding: space.md, marginBottom: space.md }}>
          <Text style={{ color: colors.green, fontWeight: '700' }}>{notice}</Text>
        </View>
      )}
      {sale.status === 'completed' && !notice && (
        <Text testID="payment-success" style={{ color: colors.green, fontWeight: '700', marginBottom: space.md }}>Payment successful</Text>
      )}

      <Card>
        <Text testID="receipt-text" style={{ fontFamily: 'Courier', fontSize: 12, color: colors.ink, lineHeight: 17 }}>
          {receiptText(db, detail)}
        </Text>
      </Card>

      <Button testID="done" variant="lime" label="Done" onPress={() => router.replace('/')} />
      <View style={{ height: 8 }} />
      <Button testID="share-receipt" variant="ghost" label="Share receipt" onPress={() => Share.share({ message: receiptText(db, detail) })} />
      <View style={{ height: 8 }} />

      {mode === 'none' && (canRefund || canVoid) && (
        <View style={{ flexDirection: 'row', gap: 8 }}>
          {canRefund && <Button style={{ flex: 1 }} testID="start-refund" variant="primary" label="Refund / return" onPress={() => setMode('refund')} />}
          {canVoid && <Button style={{ flex: 1 }} testID="start-void" variant="danger" label="Void sale" onPress={() => setMode('void')} />}
        </View>
      )}

      {mode === 'void' && (
        <Card>
          <Text style={{ fontWeight: '700', marginBottom: 6 }}>Void this sale?</Text>
          <Text style={{ color: colors.muted, marginBottom: space.md }}>All items return to stock and loyalty points are reversed.</Text>
          <Field testID="void-reason" label="Reason (optional)" value={reason} onChangeText={setReason} />
          <Button testID="confirm-void" variant="danger" label="Confirm void" onPress={doVoid} />
          <View style={{ height: 8 }} />
          <Button variant="ghost" label="Cancel" onPress={close} />
        </Card>
      )}

      {mode === 'refund' && (
        <Card>
          <Text style={{ fontWeight: '700', marginBottom: space.md }}>Choose items to return</Text>
          {items.map((i) => {
            const remaining = i.qty - i.refundedQty;
            return (
              <View key={i.id} style={{ flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', paddingVertical: 6 }}>
                <View style={{ flex: 1 }}>
                  <Text style={{ fontWeight: '600' }}>{i.name}</Text>
                  <Text style={{ color: colors.muted, fontSize: 12 }}>{remaining} of {i.qty} refundable</Text>
                </View>
                {remaining > 0 ? (
                  <Stepper testID={`refund-${i.sku}`} value={qtys[i.id] ?? 0} max={remaining} onChange={(q) => setQtys({ ...qtys, [i.id]: q })} />
                ) : (
                  <Text style={{ color: colors.muted }}>Refunded</Text>
                )}
              </View>
            );
          })}
          <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginVertical: space.md }}>
            <Text>Return items to stock</Text>
            <Switch testID="refund-restock" value={restock} onValueChange={setRestock} trackColor={{ true: colors.limeLine }} />
          </View>
          <Field testID="refund-reason" label="Reason (optional)" value={reason} onChangeText={setReason} />
          <Text testID="refund-preview" style={{ fontWeight: '800', marginBottom: space.md }}>Refund total: {formatZAR(refundTotalPreview)}</Text>
          <Button testID="confirm-refund" label="Confirm refund" disabled={refundTotalPreview <= 0} onPress={doRefund} />
          <View style={{ height: 8 }} />
          <Button variant="ghost" label="Cancel" onPress={close} />
        </Card>
      )}

      {detail.refunds.length > 0 && (
        <>
          <SectionTitle>Refund history</SectionTitle>
          {detail.refunds.map((r) => (
            <Card key={r.id} testID={`refund-card-${r.id}`}>
              <Text style={{ fontFamily: 'Courier', fontSize: 12, lineHeight: 17 }}>{refundReceiptText(db, detail, r)}</Text>
              <View style={{ height: 8 }} />
              <Button small variant="ghost" label="Share refund receipt" onPress={() => Share.share({ message: refundReceiptText(db, detail, r) })} />
            </Card>
          ))}
        </>
      )}
      {sale.status === 'voided' && (
        <Text testID="void-info" style={{ color: colors.muted, marginTop: space.md }}>Voided: {sale.voidReason}</Text>
      )}
    </ScrollView>
  );
}
