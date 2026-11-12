import { create } from 'zustand'

interface WalletStore {
    smartContractDeployed: boolean
    address: string | null
    smartAdress: string | null
    setWallet: ( data :{
        smartContractDeployed: boolean,
        address: string,
        smartAdress: string,
    }) => void
}

export const useWallet = create<WalletStore>((set) => ({
    smartContractDeployed: false,
    address: null,
    smartAdress: null,
    setWallet: ( data :{
        smartContractDeployed: boolean,
        address: string,
        smartAdress: string,
    }) => {
        set({
            ...data
        })
    }
}))
