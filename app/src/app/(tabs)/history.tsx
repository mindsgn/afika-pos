import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { FlatList, Pressable, ScrollView, Text, View } from 'react-native';
import { formatZAR } from '@/domain/money';
import { formatDateTime } from '@/domain/receipt';
import { useFocusData } from '@/hooks/useFocusData';
import { listSales } from '@/services/sales';
import { colors, radius, space } from '@/theme/colors';
import { Chip, Empty, SearchBar, StatusBadge } from '@/ui/components';
import { getDb } from '@/db/client';

const FILTERS: { key: string; label: string }[] = [
  { key: '', label: 'All' },
  { key: 'pending', label: 'Awaiting payment' },
  { key: 'completed', label: 'Completed' },
  { key: 'failed', label: 'Failed' },
  { key: 'voided', label: 'Voided' },
  { key: 'partially_refunded', label: 'Part refunded' },
  { key: 'refunded', label: 'Refunded' },
];

export default function HistoryScreen() {
  const db = getDb();
  const router = useRouter();
  const [status, setStatus] = useState('');
  const [query, setQuery] = useState('');
  const [rows] = useFocusData(() => listSales(db, { status: status || undefined, query }), [status, query]);

  return (
    <View style={{ flex: 1, padding: space.lg }}>
      <SearchBar testID="history-search" value={query} onChangeText={setQuery} placeholder="Search receipt number" />
      <View style={{ height: 44, marginTop: space.md }}>
        <ScrollView horizontal showsHorizontalScrollIndicator={false}>
          {FILTERS.map((f) => (
            <Chip key={f.key} label={f.label} active={status === f.key} onPress={() => setStatus(f.key)} testID={`filter-${f.key || 'all'}`} />
          ))}
        </ScrollView>
      </View>
      <FlatList
        data={rows}
        keyExtractor={(s) => String(s.id)}
        ListEmptyComponent={<Empty text="No transactions yet" />}
        contentContainerStyle={{ gap: space.sm, paddingBottom: space.xl }}
        renderItem={({ item }) => (
          <Pressable
            testID={`sale-row-${item.receiptNo}`}
            onPress={() => router.push(`/sale/${item.id}`)}
            style={{ backgroundColor: colors.paper, borderRadius: radius.lg, padding: space.lg, borderWidth: 1, borderColor: colors.line }}
          >
            <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' }}>
              <Text style={{ fontWeight: '800', color: colors.ink }}>{item.receiptNo}</Text>
              <StatusBadge status={item.status} />
            </View>
            <View style={{ flexDirection: 'row', justifyContent: 'space-between', marginTop: 6 }}>
              <Text style={{ color: colors.muted, fontSize: 12 }}>{formatDateTime(item.createdAt)} · {item.paymentMethod}</Text>
              <Text style={{ fontWeight: '700', color: colors.ink }}>{formatZAR(item.total)}</Text>
            </View>
          </Pressable>
        )}
      />
    </View>
  );
}
