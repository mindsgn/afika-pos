import React, { useState } from 'react';
import { ScrollView, Text, View } from 'react-native';
import { getDb } from '@/db/client';
import { useFocusData } from '@/hooks/useFocusData';
import { createCategory, deleteCategory, listCategories, renameCategory } from '@/services/catalog';
import { toast } from '@/shared/ui/molecules/Toast';
import { colors, space } from '@/theme/colors';
import { Button, Card, Field } from '@/ui/components';

export default function CategoriesScreen() {
  const db = getDb();
  const [categories, refresh] = useFocusData(() => listCategories(db));
  const [name, setName] = useState('');
  const [editing, setEditing] = useState<number | null>(null);
  const [editName, setEditName] = useState('');

  const run = (fn: () => void) => {
    try {
      fn();
      refresh();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Something went wrong');
    }
  };

  return (
    <ScrollView contentContainerStyle={{ padding: space.lg }} keyboardShouldPersistTaps="handled">
      <Card>
        <Field testID="category-name" label="New category" value={name} onChangeText={setName} />
        <Button testID="category-add" label="Add category" variant="lime" onPress={() => run(() => { createCategory(db, name); setName(''); })} />
      </Card>
      {categories.map((c) => (
        <Card key={c.id}>
          {editing === c.id ? (
            <View>
              <Field testID={`category-edit-input-${c.id}`} value={editName} onChangeText={setEditName} />
              <View style={{ flexDirection: 'row', gap: 8 }}>
                <Button small label="Save" onPress={() => run(() => { renameCategory(db, c.id, editName); setEditing(null); })} />
                <Button small variant="ghost" label="Cancel" onPress={() => setEditing(null)} />
              </View>
            </View>
          ) : (
            <View style={{ flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center' }}>
              <Text style={{ fontWeight: '700', color: colors.ink }}>{c.name}</Text>
              <View style={{ flexDirection: 'row', gap: 8 }}>
                <Button small variant="ghost" label="Rename" testID={`category-rename-${c.name}`} onPress={() => { setEditing(c.id); setEditName(c.name); }} />
                <Button small variant="ghost" label="Delete" testID={`category-delete-${c.name}`} onPress={() => run(() => deleteCategory(db, c.id))} />
              </View>
            </View>
          )}
        </Card>
      ))}
    </ScrollView>
  );
}
