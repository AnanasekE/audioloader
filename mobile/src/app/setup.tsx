import { useRouter } from "expo-router";
import { useState } from "react";
import { ActivityIndicator, KeyboardAvoidingView, Platform, Pressable, StyleSheet, Text, TextInput, View } from "react-native";
import { checkServer, normalizeUrl } from "@/server";
import { useServer } from "@/serverContext";
import { colors, font, radius } from "@/theme";

// First-launch screen (also reachable from settings): enter the server URL.
export default function Setup() {
  const { url: current, setUrl } = useServer();
  const router = useRouter();
  const [text, setText] = useState(current ?? "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function connect() {
    const url = normalizeUrl(text);
    if (!url) {
      setError("Enter the server address, e.g. https://audioloader.your-tailnet.ts.net");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await checkServer(url);
      await setUrl(url);
      router.replace("/");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not connect.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <KeyboardAvoidingView style={styles.root} behavior={Platform.OS === "ios" ? "padding" : undefined}>
      <Text style={styles.title}>Audioloader</Text>
      <Text style={styles.hint}>Address of your server (reachable over Tailscale)</Text>
      <TextInput
        style={styles.input}
        value={text}
        onChangeText={setText}
        placeholder="https://audioloader.your-tailnet.ts.net"
        placeholderTextColor={colors.fgSubtle}
        autoCapitalize="none"
        autoCorrect={false}
        keyboardType="url"
        onSubmitEditing={connect}
      />
      {error ? <Text style={styles.error}>{error}</Text> : null}
      <Pressable style={[styles.button, busy && { opacity: 0.6 }]} onPress={connect} disabled={busy}>
        {busy ? <ActivityIndicator color={colors.accentInk} /> : <Text style={styles.buttonText}>Connect</Text>}
      </Pressable>
      {current ? (
        <View style={{ marginTop: 16 }}>
          <Pressable onPress={() => router.back()}>
            <Text style={styles.link}>Cancel</Text>
          </Pressable>
        </View>
      ) : null}
    </KeyboardAvoidingView>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, backgroundColor: colors.bg, justifyContent: "center", padding: 24 },
  title: { color: colors.fg, fontSize: font.xxl, fontWeight: "700", marginBottom: 8 },
  hint: { color: colors.fgMuted, fontSize: font.base, marginBottom: 16 },
  input: {
    backgroundColor: colors.panel,
    borderColor: colors.line,
    borderWidth: 1,
    borderRadius: radius.md,
    color: colors.fg,
    fontSize: font.lg,
    padding: 12,
  },
  error: { color: colors.failed, fontSize: font.base, marginTop: 12 },
  button: { backgroundColor: colors.accent, borderRadius: radius.md, padding: 14, alignItems: "center", marginTop: 16 },
  buttonText: { color: colors.accentInk, fontWeight: "600", fontSize: font.lg },
  link: { color: colors.accent, fontSize: font.base, textAlign: "center" },
});
