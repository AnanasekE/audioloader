// Phone "More" sheet: a <dialog> opened by any [data-sheet-open] button.
// Esc closes it natively; a click on the backdrop (the dialog itself) does too.
// Listeners are delegated on document because hx-boost swaps the body.
document.addEventListener("click", (e) => {
	const opener = e.target.closest("[data-sheet-open]");
	if (opener) {
		document.getElementById(opener.dataset.sheetOpen)?.showModal();
		return;
	}
	if (e.target instanceof HTMLDialogElement && e.target.id.endsWith("-sheet")) {
		e.target.close();
	}
});
