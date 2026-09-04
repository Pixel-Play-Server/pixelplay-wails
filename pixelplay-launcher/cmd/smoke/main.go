package main

import (
	"fmt"

	"pixelplay-launcher/internal/ipc"
	"pixelplay-launcher/internal/updater"
	"pixelplay-launcher/internal/version"
)

func main() {
	_, err := ipc.Dispatch(nil, "list_profiles", "{}")
	fmt.Println("list_profiles ok=", err == nil, "err=", err)

	_, err = ipc.Dispatch(nil, "open_profile_folder", `{"profileId":"2c825a2e-350d-46a5-84cb-b4f7d90342de"}`)
	fmt.Println("open_protected err=", err)

	_, err = ipc.Dispatch(nil, "get_news_and_changelogs_command", "{}")
	fmt.Println("news removed ok=", err == nil)

	info, err := ipc.Dispatch(nil, "backend_info", "{}")
	fmt.Println("backend_info", info, err)

	fmt.Println("updater url", updater.ManifestURL(version.App))
	man, newer, err := updater.Check(version.App, nil)
	fmt.Println("updater check newer=", newer, "man=", man != nil, "err=", err)
}
