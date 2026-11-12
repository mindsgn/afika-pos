import React, { useState } from 'react';
import { ScrollView, Text } from 'react-native';
import { getDb } from '@/db/client';
import { formatZAR } from '@/domain/money';
import { useFocusData } from '@/hooks/useFocusData';
import { lowStockProducts } from '@/services/inventory';
import { profitSummary } from '@/services/reports';
import { colors, space } from '@/theme/colors';
import { Card, Chip, Row, SectionTitle } from '@/ui/components';
import { View } from 'react-native';

const DAY = 24 * 60 * 60 * 1000;

function startOfToday() {
  const d = new Date();
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

export default function InsightsScreen() {
  const db = getDb();
  const [range, setRange] = useState<'today' | '7d' | 'all'>('all');
  const from = range === 'today' ? startOfToday() : range === '7d' ? Date.now() - 7 * DAY : undefined;
  const [s] = useFocusData(() => profitSummary(db, { from }), [range]);
  const [low] = useFocusData(() => lowStockProducts(db));

  return (
    <ScrollView contentContainerStyle={{ padding: space.lg, paddingBottom: 60 }}>
      <View style={{ flexDirection: 'row', marginBottom: space.md }}>
        <Chip label="Today" active={range === 'today'} onPress={() => setRange('today')} testID="range-today" />
        <Chip label="7 days" active={range === '7d'} onPress={() => setRange('7d')} testID="range-7d" />
        <Chip label="All time" active={range === 'all'} onPress={() => setRange('all')} testID="range-all" />
      </View>

      <Card style={{ backgroundColor: colors.ink }}>
        <Text style={{ color: colors.muted, fontSize: 12, fontWeight: '600' }}>NET PROFIT</Text>
        <Text testID="profit-value" style={{ color: colors.lime, fontSize: 34, fontWeight: '800', marginVertical: 4 }}>{formatZAR(s.profit)}</Text>
        <Text style={{ color: colors.muted }}>Margin {s.marginPct}%</Text>
      </Card>

      <Card>
        <Row left="Sales" right={String(s.salesCount)} testID="sales-count" />
        <Row left="Gross revenue" right={formatZAR(s.grossRevenue)} testID="gross-revenue" />
        <Row left="Refunds" right={`-${formatZAR(s.refundedTotal)}`} testID="refunded-total" />
        <Row left="Net revenue" right={formatZAR(s.netRevenue)} />
        <Row left="Cost of goods" right={formatZAR(s.cogs)} testID="cogs" />
      </Card>

      <SectionTitle>Activity</SectionTitle>
      <Card>
        <Row left="Refund transactions" right={String(s.refundCount)} />
        <Row left="Voided sales" right={String(s.voidedCount)} testID="voided-count" />
        <Row left="Failed payments" right={String(s.failedCount)} testID="failed-count" />
        <Row left="Loyalty points issued" right={String(s.pointsIssued)} />
      </Card>

      {s.byMethod.length > 0 && (
        <>
          <SectionTitle>By payment method</SectionTitle>
          <Card>
            {s.byMethod.map((m) => (
              <Row key={m.method} left={`${m.method} · ${m.count} sale${m.count === 1 ? '' : 's'}`} right={formatZAR(m.revenue)} />
            ))}
          </Card>
        </>
      )}

      {low.length > 0 && (
        <>
          <SectionTitle>Low stock</SectionTitle>
          <Card>
            {low.map((p) => (
              <Row key={p.id} left={p.name} right={`${p.stock} left`} color="#9A6B00" />
            ))}
          </Card>
        </>
      )}
    </ScrollView>
  );
}
