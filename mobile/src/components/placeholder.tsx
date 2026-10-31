import { StyleSheet, Text, View } from "react-native";
import { colors, font } from "@/theme";

export function Placeholder({ title }: { title: string }) {
  return (
    <View style={styles.root}>
      <Text style={styles.text}>{title}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  root: { flex: 1, alignItems: "center", justifyContent: "center", backgroundColor: colors.bg },
  text: { color: colors.fgMuted, fontSize: font.lg },
});
