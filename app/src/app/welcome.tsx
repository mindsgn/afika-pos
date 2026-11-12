import { useLoginWithEmail } from '@privy-io/expo';
import { useRouter } from 'expo-router';
import React, { useState } from 'react';
import { Keyboard, KeyboardAvoidingView, Platform, Pressable, Text, View } from 'react-native';
import { getDb } from '@/db/client';
import { E2E_CODE, E2E_ENABLED, E2E_EMAIL, markE2eSignedIn } from '@/lib/e2e';
import { hasBusiness } from '@/services/business';
import { toast } from '@/shared/ui/molecules/Toast';
import { colors, radius, space } from '@/theme/colors';
import { Button, Field } from '@/ui/components';

type Step = 'email' | 'code';

const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/;

/**
 * Sign-in: enter an email, then the one-time code Privy sends. With
 * `EXPO_PUBLIC_E2E=1` the code is accepted locally so Maestro stays offline.
 */
export default function Welcome() {
  const router = useRouter();
  const { sendCode, loginWithCode, state } = useLoginWithEmail();
  const [step, setStep] = useState<Step>('email');
  const [email, setEmail] = useState('');
  const [code, setCode] = useState('');
  const busy = state.status === 'sending-code' || state.status === 'submitting-code';

  // First run goes on to business setup; a returning merchant lands on the till.
  const continueTo = () => router.replace(hasBusiness(getDb()) ? '/(tabs)' : '/onboarding');

  const requestCode = async () => {
    Keyboard.dismiss();
    const address = email.trim();
    if (!EMAIL_PATTERN.test(address)) {
      toast.error('Enter a valid email address');
      return;
    }
    if (E2E_ENABLED) {
      setStep('code');
      return;
    }
    try {
      await sendCode({ email: address });
      setStep('code');
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'Could not send the code');
    }
  };

  const verify = async () => {
    Keyboard.dismiss();
    const address = email.trim();
    const value = code.trim();
    if (value.length < 4) {
      toast.error('Enter the code from your email');
      return;
    }
    if (E2E_ENABLED) {
      markE2eSignedIn();
      continueTo();
      return;
    }
    try {
      const user = await loginWithCode({ email: address, code: value });
      if (!user) throw new Error('Sign in failed');
      continueTo();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : 'That code was not accepted');
    }
  };

  return (
    <KeyboardAvoidingView
      style={{ flex: 1, backgroundColor: colors.canvas }}
      behavior={Platform.OS === 'ios' ? 'padding' : undefined}
    >
      {/* Tapping the heading resigns the keyboard; the numeric keypad has no
          dismiss key, so the Maestro flow taps it between fields. */}
      <Pressable testID="signin-heading" onPress={Keyboard.dismiss} style={{ paddingTop: 96, paddingHorizontal: space.lg }}>
        <Text style={{ fontSize: 34, fontWeight: '800', color: colors.ink, letterSpacing: -1 }}>
          {step === 'email' ? 'Sign in to Afika' : 'Check your email'}
        </Text>
        <Text style={{ color: colors.muted, marginTop: 8, fontSize: 15, lineHeight: 22 }}>
          {step === 'email'
            ? 'Enter your email and we will send you a one-time code.'
            : `We sent a 6-digit code to ${email.trim()}.`}
        </Text>
      </Pressable>

      <View testID="signin-screen" style={{ padding: space.lg, marginTop: space.lg }}>
        {step === 'email' ? (
          <>
            <Field
              testID="signin-email"
              label="Email"
              value={email}
              onChangeText={setEmail}
              placeholder="you@business.co.za"
              keyboardType="email-address"
              autoCapitalize="none"
            />
            <Button
              testID="signin-send-code"
              variant="lime"
              disabled={busy}
              label={busy ? 'Sending…' : 'Send code'}
              onPress={requestCode}
            />
          </>
        ) : (
          <>
            <Field
              testID="signin-code"
              label="Authentication code"
              value={code}
              onChangeText={setCode}
              placeholder="123456"
              keyboardType="number-pad"
            />
            <Button
              testID="signin-verify"
              variant="lime"
              disabled={busy}
              label={busy ? 'Verifying…' : 'Verify and continue'}
              onPress={verify}
            />
            <View style={{ height: space.md }} />
            <Button
              variant="ghost"
              label="Use a different email"
              onPress={() => {
                setStep('email');
                setCode('');
              }}
            />
          </>
        )}
      </View>

      <View style={{ paddingHorizontal: space.lg, marginTop: 'auto', paddingBottom: space.xl }}>
        <View style={{ backgroundColor: colors.paper, borderRadius: radius.md, borderWidth: 1, borderColor: colors.line, padding: space.md }}>
          <Text style={{ color: colors.muted, fontSize: 12, lineHeight: 18 }}>
            {E2E_ENABLED
              ? `Test build: use ${E2E_EMAIL} and code ${E2E_CODE}.`
              : 'Secured by Privy. Your email is only used to sign you in.'}
          </Text>
        </View>
      </View>
    </KeyboardAvoidingView>
  );
}
