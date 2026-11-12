import { useLocalSearchParams, useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Image } from 'react-native';
import * as ImagePicker from 'expo-image-picker';
import { Tray } from '@/shared/ui/organisms/tray';
import { Switch, Text, View } from 'react-native';
import { getDb } from '@/db/client';
import { centsToInput, formatZAR, parseToCents } from '@/domain/money';
import { useFocusData } from '@/hooks/useFocusData';
import { createProduct, getProduct, listCategories, setProductActive, updateProduct } from '@/services/catalog';
import { costHistory } from '@/services/inventory';
import { toast } from '@/shared/ui/molecules/Toast';
import { colors, space } from '@/theme/colors';
import { Button, Card, Chip, Field, Row, SectionTitle } from '@/ui/components';
import { formatDateTime } from '@/domain/receipt';

export default function ProductForm() {
  const db = getDb();
  const router = useRouter();
  const { id } = useLocalSearchParams<{ id: string }>();
  const isNew = id === 'new';
  const existing = isNew ? undefined : getProduct(db, Number(id));
  const [categories] = useFocusData(() => listCategories(db));

  const [name, setName] = useState(existing?.name ?? '');
  const [sku, setSku] = useState(existing?.sku ?? '');
  const [categoryId, setCategoryId] = useState<number | null>(existing?.categoryId ?? null);
  const [price, setPrice] = useState(existing ? centsToInput(existing.price) : '');
  const [cost, setCost] = useState(existing ? centsToInput(existing.costPrice) : '');
  const [stock, setStock] = useState('0');
  const [reorder, setReorder] = useState(String(existing?.reorderLevel ?? 0));
  const [active, setActive] = useState(existing?.active ?? true);
  const [imageUri, setImageUri] = useState(existing?.imageUri ?? null);
  const [step, setStep] = useState(0);

  const priceC = parseToCents(price);
  const costC = parseToCents(cost || '0');
  const margin = priceC && costC != null && priceC > 0 ? Math.round(((priceC - costC) / priceC) * 100) : null;

  const chooseImage = async () => {
    const result = await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], allowsEditing: true, aspect: [1, 1], quality: 0.8 });
    if (!result.canceled) setImageUri(result.assets[0]?.uri ?? null);
  };

  const save = () => {
    if (priceC == null) return toast.error('Enter a valid price, e.g. 25.00');
    if (costC == null) return toast.error('Enter a valid cost price, e.g. 10.00');
    const reorderN = Number(reorder || 0);
    const stockN = Number(stock || 0);
    if (!Number.isInteger(reorderN) || reorderN < 0) return toast.error('Reorder level must be a whole number');
    if (!Number.isInteger(stockN) || stockN < 0) return toast.error('Opening stock must be a whole number');
    try {
      if (isNew) {
        createProduct(db, { name, sku, categoryId, price: priceC, costPrice: costC, stock: stockN, reorderLevel: reorderN, imageUri });
      } else {
        updateProduct(db, Number(id), { name, sku, categoryId, price: priceC, costPrice: costC, reorderLevel: reorderN, imageUri });
        setProductActive(db, Number(id), active);
      }
      router.back();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not save product');
    }
  };

  const history = existing ? costHistory(db, existing.id) : [];
  const nextStep = () => {
    if (step === 0 && !name.trim()) return toast.error('Enter a product name');
    if (step === 0 && !sku.trim()) return toast.error('Enter a SKU');
    if (step === 1 && priceC == null) return toast.error('Enter a valid price, e.g. 25.00');
    if (step === 1 && costC == null) return toast.error('Enter a valid cost price, e.g. 10.00');
    if (step === 1 && (!Number.isInteger(Number(reorder || 0)) || Number(reorder || 0) < 0)) return toast.error('Reorder level must be a whole number');
    if (step === 1 && isNew && (!Number.isInteger(Number(stock || 0)) || Number(stock || 0) < 0)) return toast.error('Opening stock must be a whole number');
    if (step < 2) setStep(step + 1); else save();
  };

  return (
    <View style={{ flex:  1 }}>
      <Tray defaultOpen theme="light" dismissOnBackdropPress={false} enableDragToDismiss={false}>
        <Tray.Content>
          <Tray.View id="default">
            <Tray.Header><Tray.Title>{isNew ? 'Add product' : 'Edit product'} · {step + 1} of 3</Tray.Title><Tray.Subtitle>{['Product details', 'Price & stock', 'Product image'][step]}</Tray.Subtitle></Tray.Header>
            <Tray.ScrollView contentContainerStyle={{ padding: space.lg, paddingBottom: 20 }}>
              {step === 0 ? <>
                <Field testID="product-name" label="Name" value={name} onChangeText={setName} />
                <Field testID="product-sku" label="SKU" value={sku} onChangeText={setSku} autoCapitalize="characters" />
                <Text style={{ color: colors.textSecondary, fontSize: 12, fontWeight: '600', marginBottom: 6 }}>Category</Text>
                <View style={{ flexDirection: 'row', flexWrap: 'wrap', rowGap: 8, marginBottom: space.md }}>
                  <Chip label="None" active={categoryId === null} onPress={() => setCategoryId(null)} />
                  {categories.map((c) => <Chip key={c.id} label={c.name} active={categoryId === c.id} onPress={() => setCategoryId(c.id)} testID={`product-cat-${c.name}`} />)}
                </View>
              </> : null}
              {step === 1 ? <>
                <Field testID="product-price" label="Selling price (R)" value={price} onChangeText={setPrice} keyboardType="decimal-pad" />
                <Field testID="product-cost" label="Cost price (R)" value={cost} onChangeText={setCost} keyboardType="decimal-pad" />
                {margin !== null ? <Text testID="product-margin" style={{ color: margin < 0 ? colors.red : colors.green, fontWeight: '700', marginBottom: space.md }}>Margin {margin}% · profit per unit {formatZAR((priceC ?? 0) - (costC ?? 0))}</Text> : null}
                {isNew ? <Field testID="product-stock" label="Opening stock" value={stock} onChangeText={setStock} keyboardType="number-pad" /> : null}
                <Field testID="product-reorder" label="Low-stock alert at" value={reorder} onChangeText={setReorder} keyboardType="number-pad" />
                {!isNew ? <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', marginBottom: space.md }}><Text>Available for sale</Text><Switch testID="product-active" value={active} onValueChange={setActive} trackColor={{ true: colors.limeLine }} /></View> : null}
              </> : null}
              {step === 2 ? <>
                <Text style={{ color: colors.muted, marginBottom: space.md }}>Add a photo so you and your customers can recognize this item.</Text>
                {imageUri ? <Image source={{ uri: imageUri }} style={{ width: 140, height: 140, borderRadius: 18, marginBottom: space.md }} /> : null}
                <Button variant="ghost" label={imageUri ? 'Change product image' : 'Add product image'} onPress={chooseImage} />
              </> : null}
              {existing && step === 2 ? <><Row left="Current stock" right={String(existing.stock)} /><SectionTitle>Cost price history</SectionTitle><Card>{history.length === 0 ? <Text style={{ color: colors.muted }}>No recorded costs yet</Text> : history.slice(0, 8).map((h) => <Row key={h.id} left={`${formatDateTime(h.createdAt)} · ${h.type}`} right={formatZAR(h.unitCost ?? 0)} />)}</Card></> : null}
            </Tray.ScrollView>
            <Tray.Footer>
              {step > 0 ? <Button variant="ghost" label="Back" onPress={() => setStep(step - 1)} /> : null}
              <Button testID={step === 2 ? 'save-product' : 'product-next'} variant="lime" label={step === 2 ? isNew ? 'Create product' : 'Save changes' : 'Continue'} onPress={nextStep} />
            </Tray.Footer>
          </Tray.View>
        </Tray.Content>
      </Tray>
    </View>
  );
}
