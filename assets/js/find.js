// The Find form hides its Artist, Type and Year fields in Artist mode.
(() => {
  const form = document.getElementById("find-form");
  if (!form) return;
  const extra = form.querySelector("[data-find-extra]");
  const q = form.querySelector("#find-q");
  const names = { song: "Song title", release: "Release title", artist: "Artist name" };
  form.addEventListener("change", (e) => {
    if (e.target.name !== "mode") return;
    extra.hidden = e.target.value === "artist";
    q.placeholder = names[e.target.value] ?? "";
  });
})();
