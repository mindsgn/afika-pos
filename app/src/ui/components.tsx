import { Ionicons } from '@expo/vector-icons';
import React from 'react';
import {
  KeyboardTypeOptions,
  Pressable,
  StyleProp,
  StyleSheet,
  Text,
  TextInput,
  View,
  ViewStyle,
} from 'react-native';
import { colors, radius, space } from '../theme/colors';

type ButtonVariant = 'primary' | 'lime' | 'ghost' | 'danger';

export function Button(props: {
  label: string;
  onPress: () => void;
  testID?: string;
  variant?: ButtonVariant;
  disabled?: boolean;
  small?: boolean;
  style?: StyleProp<ViewStyle>;
}) {
  const v = props.variant ?? 'primary';
  const bg =
    v === 'primary' ? colors.buttonBackground : v === 'lime' ? colors.lime : v === 'danger' ? colors.danger : 'transparent';
  const fg = v === 'primary' ? colors.buttonTextBackground : v === 'danger' ? colors.paper : colors.ink;
  return (
    <Pressable
      testID={props.testID}
      accessibilityRole="button"
      accessibilityLabel={props.label}
      onPress={props.onPress}
      disabled={props.disabled}
      style={[
        styles.button,
        { backgroundColor: bg, opacity: props.disabled ? 0.4 : 1, paddingVertical: props.small ? 8 : 14 },
        v === 'ghost' && { borderWidth: 1, borderColor: colors.border },
        props.style,
      ]}
    >
      <Text style={{ color: fg, fontWeight: '700', fontSize: props.small ? 13 : 15 }}>{props.label}</Text>
    </Pressable>
  );
}

export function Card({ children, style, testID }: { children: React.ReactNode; style?: StyleProp<ViewStyle>; testID?: string }) {
  return (
    <View testID={testID} style={[styles.card, style]}>
      {children}
    </View>
  );
}

export function Chip(props: { label: string; active?: boolean; onPress: () => void; testID?: string }) {
  return (
    <Pressable
      testID={props.testID}
      onPress={props.onPress}
      accessibilityRole="button"
      style={[styles.chip, props.active && { backgroundColor: colors.ink, borderColor: colors.ink }]}
    >
      <Text style={{ color: props.active ? colors.lime : colors.ink, fontWeight: '600', fontSize: 13 }}>{props.label}</Text>
    </Pressable>
  );
}

export function Field(props: {
  label?: string;
  value: string;
  onChangeText: (t: string) => void;
  placeholder?: string;
  keyboardType?: KeyboardTypeOptions;
  testID?: string;
  multiline?: boolean;
  autoCapitalize?: 'none' | 'sentences' | 'words' | 'characters';
  style?: StyleProp<ViewStyle>;
}) {
  return (
    <View style={[{ marginBottom: space.md }, props.style]}>
      {props.label ? <Text style={styles.label}>{props.label}</Text> : null}
      <TextInput
        testID={props.testID}
        accessibilityLabel={props.label ?? props.placeholder}
        value={props.value}
        onChangeText={props.onChangeText}
        placeholder={props.placeholder}
        placeholderTextColor={colors.muted}
        keyboardType={props.keyboardType}
        multiline={props.multiline}
        autoCapitalize={props.autoCapitalize ?? 'sentences'}
        style={[styles.input, props.multiline && { minHeight: 70, textAlignVertical: 'top' }]}
      />
    </View>
  );
}

export function SearchBar(props: { value: string; onChangeText: (t: string) => void; placeholder?: string; testID?: string }) {
  return (
    <View style={styles.search}>
      <Ionicons name="search" size={18} color={colors.muted} />
      <TextInput
        testID={props.testID}
        value={props.value}
        onChangeText={props.onChangeText}
        placeholder={props.placeholder ?? 'Search'}
        placeholderTextColor={colors.muted}
        autoCapitalize="none"
        style={{ flex: 1, marginLeft: 8, color: colors.ink, fontSize: 15 }}
      />
    </View>
  );
}

const STATUS_STYLE: Record<string, { bg: string; fg: string; label: string }> = {
  pending: { bg: '#FFF3D6', fg: '#9A6B00', label: 'Awaiting payment' },
  completed: { bg: '#E3F3EB', fg: colors.green, label: 'Completed' },
  failed: { bg: '#FBE5E6', fg: colors.red, label: 'Failed' },
  voided: { bg: colors.line, fg: colors.ink, label: 'Voided' },
  refunded: { bg: '#FBE5E6', fg: colors.red, label: 'Refunded' },
  partially_refunded: { bg: '#FFF3D6', fg: '#9A6B00', label: 'Part refunded' },
};

export function StatusBadge({ status }: { status: string }) {
  const s = STATUS_STYLE[status] ?? { bg: colors.line, fg: colors.ink, label: status };
  return (
    <View style={{ backgroundColor: s.bg, paddingHorizontal: 10, paddingVertical: 4, borderRadius: radius.pill }}>
      <Text style={{ color: s.fg, fontSize: 12, fontWeight: '700' }}>{s.label}</Text>
    </View>
  );
}

export function Stepper(props: { value: number; onChange: (n: number) => void; min?: number; max?: number; testID?: string }) {
  const min = props.min ?? 0;
  return (
    <View style={{ flexDirection: 'row', alignItems: 'center' }}>
      <Pressable
        testID={props.testID ? `${props.testID}-dec` : undefined}
        accessibilityLabel="Decrease"
        onPress={() => props.onChange(Math.max(min, props.value - 1))}
        style={styles.stepBtn}
      >
        <Ionicons name="remove" size={18} color={colors.ink} />
      </Pressable>
      <Text testID={props.testID ? `${props.testID}-value` : undefined} style={{ minWidth: 30, textAlign: 'center', fontWeight: '700' }}>
        {props.value}
      </Text>
      <Pressable
        testID={props.testID ? `${props.testID}-inc` : undefined}
        accessibilityLabel="Increase"
        onPress={() => props.onChange(props.max != null ? Math.min(props.max, props.value + 1) : props.value + 1)}
        style={styles.stepBtn}
      >
        <Ionicons name="add" size={18} color={colors.ink} />
      </Pressable>
    </View>
  );
}

export function Row({ left, right, bold, testID, color }: { left: string; right: string; bold?: boolean; testID?: string; color?: string }) {
  return (
    <View style={{ flexDirection: 'row', justifyContent: 'space-between', paddingVertical: 4 }}>
      <Text style={{ color: color ?? colors.textSecondary, fontWeight: bold ? '700' : '400', fontSize: bold ? 17 : 14 }}>{left}</Text>
      <Text testID={testID} style={{ color: color ?? colors.ink, fontWeight: bold ? '800' : '600', fontSize: bold ? 17 : 14 }}>
        {right}
      </Text>
    </View>
  );
}

export function Empty({ text }: { text: string }) {
  return (
    <View style={{ padding: space.xl, alignItems: 'center' }}>
      <Text style={{ color: colors.muted }}>{text}</Text>
    </View>
  );
}

export function ErrorBanner({ text, testID }: { text: string | null; testID?: string }) {
  if (!text) return null;
  return (
    <View testID={testID} style={styles.error}>
      <Text style={{ color: colors.red, fontWeight: '600' }}>{text}</Text>
    </View>
  );
}

export function SectionTitle({ children }: { children: string }) {
  return <Text style={styles.section}>{children}</Text>;
}

const styles = StyleSheet.create({
  button: { borderRadius: radius.md, alignItems: 'center', justifyContent: 'center', paddingHorizontal: 18 },
  card: { backgroundColor: colors.paper, borderRadius: radius.lg, padding: space.lg, borderWidth: 1, borderColor: colors.line, marginBottom: space.md },
  chip: { borderWidth: 1, borderColor: colors.border, backgroundColor: colors.paper, paddingHorizontal: 14, paddingVertical: 8, borderRadius: radius.pill, marginRight: 8 },
  label: { color: colors.textSecondary, fontSize: 12, fontWeight: '600', marginBottom: 6 },
  input: { backgroundColor: colors.paper, borderWidth: 1, borderColor: colors.border, borderRadius: radius.md, paddingHorizontal: 14, paddingVertical: 12, fontSize: 15, color: colors.ink },
  search: { flexDirection: 'row', alignItems: 'center', backgroundColor: colors.paper, borderWidth: 1, borderColor: colors.border, borderRadius: radius.pill, paddingHorizontal: 14, paddingVertical: 10 },
  stepBtn: { width: 32, height: 32, borderRadius: 16, backgroundColor: colors.canvas, alignItems: 'center', justifyContent: 'center' },
  error: { backgroundColor: '#FBE5E6', borderRadius: radius.md, padding: space.md, marginBottom: space.md },
  section: { fontSize: 13, fontWeight: '700', color: colors.textSecondary, textTransform: 'uppercase', marginTop: space.md, marginBottom: space.sm, letterSpacing: 0.6 },
});
