import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { FlatList, Pressable, ScrollView, Text, View } from 'react-native';
import { getDb } from '@/db/client';
import { formatZAR } from '@/domain/money';
import { useFocusData } from '@/hooks/useFocusData';
import { listCategories, listProducts } from '@/services/catalog';
import { colors, radius, space } from '@/theme/colors';
import { Button, Chip, Empty, SearchBar } from '@/ui/components';

export default function ProductsScreen() {
  const db = getDb();
  const router = useRouter();
  const [query, setQuery] = useState('');
  const [categoryId, setCategoryId] = useState<number | null>(null);
  const [categories] = useFocusData(() => listCategories(db));
  const [rows] = useFocusData(() => listProducts(db, { query, categoryId, includeInactive: true }), [query, categoryId]);

  return (
    <View style={{ flex: 1, padding: space.lg }}>
      <View style={{ flexDirection: 'row', gap: 8, marginBottom: space.md }}>
        <Button style={{ flex: 1 }} testID="add-product" variant="lime" label="+ New product" onPress={() => router.push('/product/new')} />
        <Button testID="manage-categories" variant="ghost" label="Categories" onPress={() => router.push('/categories')} />
      </View>
      <SearchBar testID="product-search" value={query} onChangeText={setQuery} placeholder="Search products" />
      <View style={{ height: 44, marginTop: space.md }}>
        <ScrollView horizontal showsHorizontalScrollIndicator={false}>
          <Chip label="All" active={categoryId === null} onPress={() => setCategoryId(null)} />
          {categories.map((c) => (
            <Chip key={c.id} label={c.name} active={categoryId === c.id} onPress={() => setCategoryId(c.id)} />
          ))}
        </ScrollView>
      </View>
      <FlatList
        data={rows}
        keyExtractor={(p) => String(p.id)}
        ListEmptyComponent={<Empty text="No products found" />}
        contentContainerStyle={{ gap: space.sm, paddingBottom: space.xl }}
        renderItem={({ item }) => {
          const margin = item.price > 0 ? Math.round(((item.price - item.costPrice) / item.price) * 100) : 0;
          return (
            <Pressable
              testID={`manage-${item.sku}`}
              onPress={() => router.push(`/product/${item.id}`)}
              style={{ backgroundColor: colors.paper, borderRadius: radius.lg, padding: space.lg, borderWidth: 1, borderColor: colors.line, opacity: item.active ? 1 : 0.5 }}
            >
              <View style={{ flexDirection: 'row', justifyContent: 'space-between' }}>
                <Text style={{ fontWeight: '800', color: colors.ink }}>{item.name}{item.active ? '' : ' (archived)'}</Text>
                <Text style={{ fontWeight: '700' }}>{formatZAR(item.price)}</Text>
              </View>
              <Text style={{ color: colors.muted, fontSize: 12, marginTop: 4 }}>
                {item.sku} · {item.categoryName ?? 'Uncategorised'} · cost {formatZAR(item.costPrice)} · margin {margin}% · stock {item.stock}
              </Text>
            </Pressable>
          );
        }}
      />
    </View>
  );
}
