import React, { useState } from 'react';
import { FlatList, Pressable, Text, View } from 'react-native';
import { getDb } from '@/db/client';
import { centsToInput, formatZAR, parseToCents } from '@/domain/money';
import { formatDateTime } from '@/domain/receipt';
import { useFocusData } from '@/hooks/useFocusData';
import { listProducts } from '@/services/catalog';
import { adjustStock, inventoryValuation, listMovements } from '@/services/inventory';
import { toast } from '@/shared/ui/molecules/Toast';
import { colors, radius, space } from '@/theme/colors';
import { Button, Card, Chip, Empty, Field, Row, SearchBar } from '@/ui/components';

export default function StockScreen() {
  const db = getDb();
  const [view, setView] = useState<'levels' | 'movements'>('levels');
  const [query, setQuery] = useState('');
  const [filterId, setFilterId] = useState<number | null>(null);
  const [openId, setOpenId] = useState<number | null>(null);
  const [kind, setKind] = useState<'restock' | 'adjustment'>('restock');
  const [qty, setQty] = useState('');
  const [cost, setCost] = useState('');
  const [note, setNote] = useState('');

  const [products, refresh] = useFocusData(() => listProducts(db, { query, includeInactive: true }), [query]);
  const [movements, refreshMoves] = useFocusData(() => listMovements(db, { productId: filterId ?? undefined }), [filterId]);
  const [valuation, refreshVal] = useFocusData(() => inventoryValuation(db));

  const save = (productId: number) => {
    const n = Number(qty);
    try {
      const unitCost = kind === 'restock' && cost.trim() ? parseToCents(cost) : null;
      if (kind === 'restock' && cost.trim() && unitCost == null) throw new Error('Enter a valid unit cost, e.g. 12.50');
      adjustStock(db, { productId, delta: n, kind, note, unitCost });
      setOpenId(null);
      setQty('');
      setCost('');
      setNote('');
      refresh();
      refreshMoves();
      refreshVal();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not update stock');
    }
  };

  return (
    <View style={{ flex: 1, padding: space.lg }}>
      <View style={{ flexDirection: 'row', marginBottom: space.md }}>
        <Chip label="Levels" active={view === 'levels'} onPress={() => setView('levels')} testID="seg-levels" />
        <Chip label="Movements" active={view === 'movements'} onPress={() => setView('movements')} testID="seg-movements" />
      </View>

      {view === 'levels' ? (
        <FlatList
          data={products}
          keyExtractor={(p) => String(p.id)}
          ListHeaderComponent={
            <View>
              <Card style={{ backgroundColor: colors.ink }}>
                <Row color={colors.muted} left="Units on hand" right={String(valuation.units)} testID="val-units" />
                <Row color={colors.muted} left="Stock at cost" right={formatZAR(valuation.costValue)} testID="val-cost" />
                <Row color={colors.lime} bold left="Stock at retail" right={formatZAR(valuation.retailValue)} />
              </Card>
              <SearchBar testID="stock-search" value={query} onChangeText={setQuery} placeholder="Search stock" />
              <View style={{ height: space.md }} />
            </View>
          }
          ListEmptyComponent={<Empty text="No products" />}
          contentContainerStyle={{ gap: space.sm, paddingBottom: space.xl }}
          renderItem={({ item }) => {
            const low = item.reorderLevel > 0 && item.stock <= item.reorderLevel;
            const open = openId === item.id;
            return (
              <View style={{ backgroundColor: colors.paper, borderRadius: radius.lg, padding: space.lg, borderWidth: 1, borderColor: low ? '#E6C36A' : colors.line }}>
                <Pressable testID={`stock-row-${item.sku}`} onPress={() => { setOpenId(open ? null : item.id); setCost(centsToInput(item.costPrice)); }}>
                  <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
                    <Text style={{ fontWeight: '800', color: colors.ink }}>{item.name}</Text>
                    <Text testID={`stock-qty-${item.sku}`} style={{ fontWeight: '800', color: low ? '#9A6B00' : colors.ink }}>{item.stock}</Text>
                  </View>
                  <Text style={{ color: colors.muted, fontSize: 12, marginTop: 2 }}>
                    {item.sku} · cost {formatZAR(item.costPrice)}{low ? ' · LOW STOCK' : ''}
                  </Text>
                </Pressable>
                {open && (
                  <View style={{ marginTop: space.md }}>
                    <View style={{ flexDirection: 'row', marginBottom: space.md }}>
                      <Chip label="Restock" active={kind === 'restock'} onPress={() => setKind('restock')} testID="kind-restock" />
                      <Chip label="Adjust (+/-)" active={kind === 'adjustment'} onPress={() => setKind('adjustment')} testID="kind-adjustment" />
                    </View>
                    <Field testID="adjust-qty" label={kind === 'restock' ? 'Quantity received' : 'Change (e.g. -2 for damaged)'} value={qty} onChangeText={setQty} keyboardType="numbers-and-punctuation" />
                    {kind === 'restock' && <Field testID="adjust-cost" label="Unit cost (R)" value={cost} onChangeText={setCost} keyboardType="decimal-pad" />}
                    <Field testID="adjust-note" label="Note (optional)" value={note} onChangeText={setNote} />
                    <Button testID="adjust-save" variant="lime" label="Save" onPress={() => save(item.id)} />
                  </View>
                )}
              </View>
            );
          }}
        />
      ) : (
        <FlatList
          data={movements}
          keyExtractor={(m) => String(m.id)}
          ListHeaderComponent={
            filterId ? (
              <View style={{ marginBottom: space.md, flexDirection: 'row' }}>
                <Chip label="Clear product filter ✕" active onPress={() => setFilterId(null)} />
              </View>
            ) : null
          }
          ListEmptyComponent={<Empty text="No stock movements yet" />}
          contentContainerStyle={{ gap: space.sm, paddingBottom: space.xl }}
          renderItem={({ item }) => (
            <Pressable
              testID={`movement-${item.id}`}
              onPress={() => setFilterId(item.productId)}
              style={{ backgroundColor: colors.paper, borderRadius: radius.lg, padding: space.md, borderWidth: 1, borderColor: colors.line }}
            >
              <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
                <Text style={{ fontWeight: '700', color: colors.ink }}>{item.productName}</Text>
                <Text style={{ fontWeight: '800', color: item.quantity > 0 ? colors.green : colors.red }}>
                  {item.quantity > 0 ? `+${item.quantity}` : item.quantity}
                </Text>
              </View>
              <Text style={{ color: colors.muted, fontSize: 12, marginTop: 2 }}>
                {item.type} · balance {item.balanceAfter} · {formatDateTime(item.createdAt)}
                {item.reference ? ` · ${item.reference}` : ''}
                {item.unitCost != null ? ` · cost ${formatZAR(item.unitCost)}` : ''}
              </Text>
            </Pressable>
          )}
        />
      )}
    </View>
  );
}
