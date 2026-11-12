import { useEffect, useState } from "react";
import { createPublicClient, createWalletClient, custom, http } from "viem";
import { base } from "viem/chains";

import {
  createKernelAccount,
  createKernelAccountClient,
  createZeroDevPaymasterClient,
} from "@zerodev/sdk";

import { signerToEcdsaValidator } from "@zerodev/ecdsa-validator";
import { getEntryPoint, KERNEL_V3_1 } from "@zerodev/sdk/constants";

type Eip1193Provider = {
  request: (args: { method: string; params?: unknown[] }) => Promise<any>;
};

type ProviderLike = {
  getProvider?: () => Promise<Eip1193Provider> | Eip1193Provider;
  request?: Eip1193Provider["request"];
};

const chain = base;
const entryPoint = getEntryPoint("0.7");
const kernelVersion = KERNEL_V3_1;

const resolveProvider = async (provider?: ProviderLike): Promise<Eip1193Provider | undefined> => {
  if (!provider) return undefined;
  if (typeof provider.getProvider === "function") {
    const resolved = await provider.getProvider();
    if (resolved) {
      return resolved;
    }
  }

  if (typeof provider.request === "function") {
    return provider as Eip1193Provider;
  }

  return undefined;
};

const requireProvider = (provider?: Eip1193Provider): Eip1193Provider => {
  if (!provider) {
    throw new Error("Unsupported provider");
  }
  return provider;
};

const requireProjectId = (projectId?: string): string => {
  if (!projectId) {
    throw new Error("Missing EXPO_PUBLIC_ZERODEV_ID");
  }
  return projectId;
};

export function useKernelClient(provider?: ProviderLike) {
  const [kernelAddress, setKernelAddress] = useState<`0x${string}` | null>(null);
  const [kernelClient, setKernelClient] = useState<any>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    let active = true;

    async function setupKernel() {
      try {
        setLoading(true);

        const resolvedProvider = requireProvider(await resolveProvider(provider));

        const projectId = requireProjectId(process.env.EXPO_PUBLIC_ZERODEV_ID);

        const ZERODEV_RPC = `https://rpc.zerodev.app/api/v3/${projectId}/chain/8453`;

        const publicClient = createPublicClient({
          chain,
          transport: http(),
        });

        const accounts = await resolvedProvider.request({
          method: "eth_requestAccounts",
        });

        const ownerAddress = accounts[0] as `0x${string}`;

        const walletClient = createWalletClient({
          account: ownerAddress,
          chain,
          transport: custom(resolvedProvider),
        });

        const ecdsaValidator = await signerToEcdsaValidator(publicClient, {
          signer: walletClient,
          entryPoint,
          kernelVersion,
        });

        const account = await createKernelAccount(publicClient, {
          plugins: {
            sudo: ecdsaValidator,
          },
          entryPoint,
          kernelVersion,
        });

        const paymasterClient = createZeroDevPaymasterClient({
          chain,
          transport: http(ZERODEV_RPC),
        });

        const client = createKernelAccountClient({
          account,
          chain,
          client: publicClient,
          bundlerTransport: http(ZERODEV_RPC),
          paymaster: {
            getPaymasterData(userOperation) {
              return paymasterClient.sponsorUserOperation({
                userOperation,
              });
            },
          },
        });

        if (active) {
          setKernelAddress(account.address);
          setKernelClient(client);
        }
      } catch (error) {
        console.log("Kernel setup error:", error);
      }

      if (active) {
        setLoading(false);
      }
    }

    setupKernel();

    return () => {
      active = false;
    };
  }, [provider]);

  return {
    kernelAddress,
    kernelClient,
    loading,
  };
}