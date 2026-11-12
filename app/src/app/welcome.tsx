import { OAuthProviderID, useLoginWithOAuth } from '@privy-io/expo';
import { useRouter } from 'expo-router';
import { Platform } from 'react-native';
import { toast } from '@/shared/ui/molecules/Toast';
import WelcomeScreenV4 from '@/shared/ui/blocks/welcome-v4';
import type { IWelcomeAction } from '@/shared/ui/blocks/welcome-v4/types';

export default function Welcome() {
  const router = useRouter();
  const { login } = useLoginWithOAuth();
  const provider: OAuthProviderID = Platform.OS === 'ios' ? 'apple' : 'google';
  const action: IWelcomeAction = {
    key: provider,
    label: provider === 'apple' ? 'Sign in with Apple' : 'Sign in with Google',
    icon: provider === 'apple' ? 'apple' : undefined,
    variant: 'primary',
  };
  return (
    <WelcomeScreenV4 actions={[action]} legalPrefix="Afika POS · Payments made simple" legalLinks={[]} legalSuffix="" onActionPress={async () => {
      try {
        await login({ provider });
        router.replace('/onboarding');
      } catch (e) {
        toast.error(e instanceof Error ? e.message : 'Sign in failed. Please try again.');
      }
    }} />
  );
}
