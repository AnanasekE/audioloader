import { Platform } from "react-native";
import * as SecureStore from "expo-secure-store";

// Small key/value store for secrets and settings: secure-store on a phone,
// localStorage on web (only used by the web smoke build).
export async function getItem(key: string): Promise<string | null> {
  if (Platform.OS === "web") {
    try {
      return globalThis.localStorage?.getItem(key) ?? null;
    } catch {
      return null;
    }
  }
  return SecureStore.getItemAsync(key);
}

export async function setItem(key: string, value: string): Promise<void> {
  if (Platform.OS === "web") {
    try {
      globalThis.localStorage?.setItem(key, value);
    } catch {}
    return;
  }
  await SecureStore.setItemAsync(key, value);
}

export async function removeItem(key: string): Promise<void> {
  if (Platform.OS === "web") {
    try {
      globalThis.localStorage?.removeItem(key);
    } catch {}
    return;
  }
  await SecureStore.deleteItemAsync(key);
}
