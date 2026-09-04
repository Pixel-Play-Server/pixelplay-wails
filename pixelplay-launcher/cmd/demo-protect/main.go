package main

import (
	"fmt"
	"os"
	"path/filepath"

	"pixelplay-launcher/internal/notify"
	"pixelplay-launcher/internal/packs"
	"pixelplay-launcher/internal/paths"
	"pixelplay-launcher/internal/profiles"
	"pixelplay-launcher/internal/protect"
)

func main() {
	fmt.Println("Data dir:", paths.DataDir())
	all, err := profiles.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error perfiles:", err)
		os.Exit(1)
	}

	fmt.Printf("\nPerfiles (%d):\n", len(all))
	for _, p := range all {
		pack := "-"
		if p.SelectedPixelplayPackID != nil {
			pack = *p.SelectedPixelplayPackID
			if ok, _ := packs.IsProtectedByAuthor(pack); ok {
				pack += " [PROTECTED]"
			}
		}
		fmt.Printf("  • %s  pack=%s\n", p.Name, pack)

		err := protect.AssertProfileFolderOpenable(p.ID)
		if err != nil {
			fmt.Printf("    ✗ abrir carpeta BLOQUEADO: %v\n", err)
			notify.AuthorBlocked(p.Name + " (demo)")
			mods := filepath.Join(paths.ProfileDir(p.Path), "mods")
			if err2 := protect.AssertPathOpenable(mods); err2 != nil {
				fmt.Printf("    ✗ abrir mods/ BLOQUEADO: %v\n", err2)
			}
			continue
		}
		fmt.Println("    ✓ abrir carpeta permitido")
	}
	fmt.Println("\nSi tenés notify-send, deberías ver notificaciones de los perfiles bloqueados.")
}
