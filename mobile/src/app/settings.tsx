import { useRouter } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, Pressable, ScrollView, StyleSheet, Text, TextInput, View } from "react-native";
import { lockVault, unlockErrorMessage, unlockVault } from "@/api/vault";
import { useMode } from "@/modeProvider";
import { useServer } from "@/serverContext";
import { colors, font, radius } from "@/theme";

// Settings: the server address, and the vault (secret mode) when the server
// has one.
export default function Settings() {
  const router = useRouter();
  const { url } = useServer();
  const { secret, vault, reset, loading } = useMode();
  const [pass, setPass] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function unlock() {
    if (!pass) return;
    setBusy(true);
    setError("");
    try {
      await unlockVault(pass, reset);
      setPass("");
      router.back();
    } catch (e) {
      setError(unlockErrorMessage(e));
    } finally {
      setBusy(false);
    }
  }

  async function lock() {
    setBusy(true);
    setError("");
    try {
      await lockVault(reset);
      router.back();
    } catch {
      setError("Locked in the app, but the server did not answer. It may still be unlocked there.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <ScrollView style={styles.root} contentContainerStyle={{ padding: 16 }} keyboardShouldPersistTaps="handled">
      <Text style={styles.label}>Server</Text>
      <Text style={styles.value}>{url}</Text>
      <Pressable style={styles.secondary} onPress={() => router.push("/setup")}>
        <Text style={styles.secondaryText}>Change server</Text>
      </Pressable>

      {loading ? (
        <ActivityIndicator color={colors.accent} style={{ marginTop: 24 }} />
      ) : vault ? (
        <View>
          <Text style={[styles.label, { marginTop: 32 }]}>Vault</Text>
          {secret ? (
            <>
              <Text style={styles.value}>Unlocked. Secret tracks are shown.</Text>
              <Pressable style={styles.button} onPress={lock} disabled={busy}>
                <Text style={styles.buttonText}>Lock</Text>
              </Pressable>
            </>
          ) : (
            <>
              <TextInput
                style={styles.input}
                value={pass}
                onChangeText={setPass}
                placeholder="Passphrase"
                placeholderTextColor={colors.fgSubtle}
                secureTextEntry
                autoCapitalize="none"
                autoCorrect={false}
                onSubmitEditing={unlock}
              />
              <Pressable style={[styles.button, busy && { opacity: 0.6 }]} onPress={unlock} disabled={busy}>
                <Text style={styles.buttonText}>Unlock vault</Text>
              </Pressable>
            </>
          )}
          {error ? <Text style={styles.error}>{error}</Text> : null}
        </View>
      ) : null}
    </ScrollView>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.bg },
  label: { color: colors.fgMuted, fontSize: font.sm, fontWeight: "600", textTransform: "uppercase", marginBottom: 6 },
  value: { color: colors.fg, fontSize: font.base, marginBottom: 12 },
  input: { backgroundColor: colors.panel, borderColor: colors.line, borderWidth: 1, borderRadius: radius.md, color: colors.fg, fontSize: font.lg, padding: 12 },
  button: { backgroundColor: colors.accent, borderRadius: radius.md, padding: 14, alignItems: "center", marginTop: 12 },
  buttonText: { color: colors.accentInk, fontWeight: "600", fontSize: font.lg },
  secondary: { borderColor: colors.line, borderWidth: 1, borderRadius: radius.md, padding: 12, alignItems: "center" },
  secondaryText: { color: colors.fg, fontSize: font.base },
  error: { color: colors.failed, fontSize: font.base, marginTop: 12 },
});
