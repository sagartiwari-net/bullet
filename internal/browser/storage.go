package browser

import (
	"net/url"
	"os"
	"path/filepath"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/rod/lib/utils"
)

const storageTypesAll = "cookies,local_storage,session_storage,indexeddb,cache_storage,service_workers"

func siteOrigin(loginURL, override string) string {
	if override != "" {
		return override
	}
	u, err := url.Parse(loginURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func freshUserDataDir() string {
	return filepath.Join(os.TempDir(), "bullet-checker", utils.RandString(10))
}

func withFreshProfile(l *launcher.Launcher) *launcher.Launcher {
	return l.UserDataDir(freshUserDataDir())
}

func clearAllStorage(client proto.Client, page *rod.Page, origin string) {
	_ = proto.NetworkClearBrowserCookies{}.Call(client)
	_ = proto.StorageClearCookies{}.Call(client)
	if origin != "" {
		_ = proto.StorageClearDataForOrigin{
			Origin:       origin,
			StorageTypes: storageTypesAll,
		}.Call(client)
	}
	if page != nil {
		_, _ = page.Eval(`() => {
			try { localStorage.clear(); } catch (e) {}
			try { sessionStorage.clear(); } catch (e) {}
		}`)
	}
}
