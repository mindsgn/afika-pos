import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { FlatList, Pressable, ScrollView, Text, View } from 'react-native';
import { getDb } from '@/db/client';
import { formatZAR } from '@/domain/money';
import { useFocusData } from '@/hooks/useFocusData';
import { listCategories, listProducts } from '@/services/catalog';
import { toast } from '@/shared/ui/molecules/Toast';
import { useCart } from '@/state/cart';
import { colors, radius, space } from '@/theme/colors';
import { Chip, Empty, SearchBar } from '@/ui/components';

export default function SellScreen() {
  const db = getDb();
  const router = useRouter();
  const cart = useCart();
  const [query, setQuery] = useState('');
  const [categoryId, setCategoryId] = useState<number | null>(null);

  const [categories] = useFocusData(() => listCategories(db));
  const [products] = useFocusData(() => listProducts(db, { query, categoryId }), [query, categoryId]);

  const addItem = (item: (typeof products)[number]) => {
    const err = cart.add(item);
    if (err) toast.error(err);
  };

  return (
    <View style={{ flex: 1, padding: space.lg }}>
      <SearchBar testID="search-input" value={query} onChangeText={setQuery} placeholder="Search name, SKU or category" />
      <View style={{ height: 44, marginTop: space.md }}>
        <ScrollView horizontal showsHorizontalScrollIndicator={false}>
          <Chip label="All" active={categoryId === null} onPress={() => setCategoryId(null)} testID="cat-all" />
          {categories.map((c) => (
            <Chip key={c.id} label={c.name} active={categoryId === c.id} onPress={() => setCategoryId(c.id)} testID={`cat-${c.id}`} />
          ))}
        </ScrollView>
      </View>
      <FlatList
        data={products}
        keyExtractor={(p) => String(p.id)}
        numColumns={2}
        columnWrapperStyle={{ gap: space.md }}
        contentContainerStyle={{ gap: space.md, paddingBottom: 100 }}
        ListEmptyComponent={<Empty text="No products match your search" />}
        renderItem={({ item }) => {
          const inCart = cart.lines.find((l) => l.productId === item.id)?.qty ?? 0;
          const out = item.stock <= 0;
          return (
            <Pressable
              testID={`product-${item.sku}`}
              accessibilityLabel={`Add ${item.name}`}
              disabled={out}
              onPress={() => addItem(item)}
              style={{
                flex: 1,
                backgroundColor: colors.paper,
                borderRadius: radius.lg,
                padding: space.lg,
                borderWidth: 1,
                borderColor: inCart ? colors.limeLine : colors.line,
                opacity: out ? 0.45 : 1,
              }}
            >
              <Text style={{ color: colors.muted, fontSize: 11, fontWeight: '600' }}>{item.categoryName ?? 'Uncategorised'}</Text>
              <Text style={{ color: colors.ink, fontWeight: '700', fontSize: 16, marginTop: 4 }} numberOfLines={2}>
                {item.name}
              </Text>
              <Text style={{ color: colors.ink, fontWeight: '800', fontSize: 18, marginTop: 8 }}>{formatZAR(item.price)}</Text>
              <Text style={{ color: out ? colors.red : item.reorderLevel > 0 && item.stock <= item.reorderLevel ? '#9A6B00' : colors.muted, fontSize: 12, marginTop: 2 }}>
                {out ? 'Out of stock' : `${item.stock} in stock`}
              </Text>
              {inCart > 0 && (
                <View style={{ position: 'absolute', top: 10, right: 10, backgroundColor: colors.lime, borderRadius: radius.pill, paddingHorizontal: 8, paddingVertical: 2 }}>
                  <Text style={{ fontWeight: '800', fontSize: 12 }}>{inCart}</Text>
                </View>
              )}
            </Pressable>
          );
        }}
      />
      {cart.lines.length > 0 && (
        <Pressable
          testID="cart-bar"
          accessibilityLabel="View cart"
          onPress={() => router.push('/cart')}
          style={{
            position: 'absolute', left: space.lg, right: space.lg, bottom: space.lg,
            backgroundColor: colors.ink, borderRadius: radius.lg, padding: space.lg,
            flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center',
          }}
        >
          <Text testID="cart-bar-count" style={{ color: colors.paper, fontWeight: '600' }}>
            {cart.totals.itemCount} item{cart.totals.itemCount === 1 ? '' : 's'}
          </Text>
          <Text style={{ color: colors.lime, fontWeight: '800', fontSize: 16 }}>View cart · {formatZAR(cart.totals.subtotal)}</Text>
        </Pressable>
      )}
    </View>
  );
}
