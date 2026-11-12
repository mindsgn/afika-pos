import { useRouter } from 'expo-router';
import * as ImagePicker from 'expo-image-picker';
import React, { useState } from 'react';
import { Image, KeyboardAvoidingView, Platform, Text, View } from 'react-native';
import { getDb } from '@/db/client';
import { getBusiness, saveBusiness } from '@/services/business';
import { toast } from '@/shared/ui/molecules/Toast';
import { colors, radius, space } from '@/theme/colors';
import { Button, Field } from '@/ui/components';
import { Tray } from '@/shared/ui/organisms/tray';

export default function OnboardingScreen() {
  const db = getDb();
  const router = useRouter();
  const existing = getBusiness(db);
  const [step, setStep] = useState(0);
  const [name, setName] = useState(existing?.name ?? '');
  const [website, setWebsite] = useState(existing?.website ?? '');
  const [address, setAddress] = useState(existing?.address ?? '');
  const [logoUri, setLogoUri] = useState(existing?.logoUri ?? null);
  const [busy, setBusy] = useState(false);

  const chooseLogo = async () => {
    const result = await ImagePicker.launchImageLibraryAsync({ mediaTypes: ['images'], allowsEditing: true, aspect: [1, 1], quality: 0.8 });
    if (!result.canceled) setLogoUri(result.assets[0]?.uri ?? null);
  };
  const finish = () => {
    setBusy(true);
    try { saveBusiness(db, { name, website, address, logoUri }); router.replace('/(tabs)'); }
    catch (e) { toast.error(e instanceof Error ? e.message : 'Could not save your business'); setStep(name.trim().length < 2 ? 0 : 2); }
    finally { setBusy(false); }
  };
  const advance = () => {
    if (step === 0 && name.trim().length < 2) return toast.error('Enter your business name');
    if (step === 1 && website.trim() && !/^([\w-]+\.)+[\w-]{2,}(\/.*)?$/.test(website.trim().replace(/^https?:\/\//, ''))) return toast.error('Enter a valid website');
    if (step < 2) setStep(step + 1); else finish();
  };
  const titles = ['Your business', 'Online presence', 'Location & logo'];
  return (
    <KeyboardAvoidingView style={{ flex: 1, backgroundColor: colors.paper }} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <View style={{ flex: 1, justifyContent: 'center', padding: space.lg }}>
        <Text style={{ color: colors.muted, fontWeight: '700', marginBottom: 8 }}>AFIKA POS · STEP {step + 1} OF 3</Text>
        <Text style={{ fontSize: 30, fontWeight: '800', color: colors.ink }}>{existing ? 'Business details' : 'Set up your shop'}</Text>
        <Text style={{ color: colors.muted, marginTop: 6 }}>A few details help Afika create polished receipts for your customers.</Text>
      </View>
      <Tray defaultOpen theme="light" dismissOnBackdropPress={false} enableDragToDismiss={false}>
        <Tray.Content>
          <Tray.View id="default">
            <Tray.Header><Tray.Title>{titles[step]}</Tray.Title><Tray.Subtitle>Your details stay on this device.</Tray.Subtitle></Tray.Header>
            <Tray.ScrollView contentContainerStyle={{ padding: space.lg }}>
              {step === 0 ? <Field testID="business-name" label="Business name" value={name} onChangeText={setName} placeholder="e.g. Afika Coffee" autoCapitalize="words" /> : null}
              {step === 1 ? <Field testID="business-website" label="Website (optional)" value={website} onChangeText={setWebsite} placeholder="afika.app" autoCapitalize="none" keyboardType="url" /> : null}
              {step === 2 ? <>
                <Field testID="business-address" label="Business address" value={address} onChangeText={setAddress} placeholder="12 Long Street, Cape Town" multiline />
                <Text style={{ color: colors.ink, fontWeight: '700', marginBottom: 8 }}>Business logo (optional)</Text>
                <View style={{ flexDirection: 'row', alignItems: 'center', gap: 12, marginBottom: space.md }}>
                  {logoUri ? <Image source={{ uri: logoUri }} style={{ width: 56, height: 56, borderRadius: radius.md }} /> : null}
                  <Button small variant="ghost" label={logoUri ? 'Change logo' : 'Add business logo'} onPress={chooseLogo} />
                </View>
              </> : null}
            </Tray.ScrollView>
            <Tray.Footer>
              {step > 0 ? <Button variant="ghost" label="Back" onPress={() => setStep(step - 1)} /> : null}
              <Button testID={step === 2 ? 'onboarding-save' : 'onboarding-next'} variant="lime" disabled={busy} label={busy ? 'Saving…' : step === 2 ? 'Start selling' : 'Continue'} onPress={advance} />
            </Tray.Footer>
          </Tray.View>
        </Tray.Content>
      </Tray>
    </KeyboardAvoidingView>
  );
}
