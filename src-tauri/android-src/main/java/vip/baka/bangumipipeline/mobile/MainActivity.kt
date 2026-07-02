package vip.baka.bangumipipeline.mobile

import android.os.Bundle
import android.os.SystemClock
import android.webkit.WebView
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.core.view.WindowCompat

class MainActivity : TauriActivity() {
  override val handleBackNavigation: Boolean = false

  private var lastBackPressedAt = 0L
  private var exitToast: Toast? = null

  override fun onCreate(savedInstanceState: Bundle?) {
    WindowCompat.setDecorFitsSystemWindows(window, true)
    super.onCreate(savedInstanceState)
    WindowCompat.setDecorFitsSystemWindows(window, true)
  }

  override fun onWebViewCreate(webView: WebView) {
    super.onWebViewCreate(webView)
    webView.settings.mediaPlaybackRequiresUserGesture = false

    onBackPressedDispatcher.addCallback(
      this,
      object : OnBackPressedCallback(true) {
        override fun handleOnBackPressed() {
          if (webView.canGoBack()) {
            webView.goBack()
            return
          }

          val now = SystemClock.elapsedRealtime()
          if (now - lastBackPressedAt <= EXIT_CONFIRM_WINDOW_MS) {
            exitToast?.cancel()
            finish()
            return
          }

          lastBackPressedAt = now
          exitToast?.cancel()
          exitToast = Toast.makeText(
            this@MainActivity,
            "再按一次退出 BakaVip2",
            Toast.LENGTH_SHORT,
          )
          exitToast?.show()
        }
      },
    )
  }

  override fun onDestroy() {
    exitToast?.cancel()
    exitToast = null
    super.onDestroy()
  }

  private companion object {
    private const val EXIT_CONFIRM_WINDOW_MS = 3_000L
  }
}
