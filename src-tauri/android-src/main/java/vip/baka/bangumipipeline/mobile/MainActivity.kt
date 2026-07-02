package vip.baka.bangumipipeline.mobile

import android.os.Build
import android.os.Bundle
import android.os.SystemClock
import android.view.View
import android.webkit.WebView
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.core.view.WindowCompat

class MainActivity : TauriActivity() {
  override val handleBackNavigation: Boolean = false

  private var lastBackPressedAt = 0L
  private var exitToast: Toast? = null

  override fun onCreate(savedInstanceState: Bundle?) {
    applyNormalSystemBars()
    super.onCreate(savedInstanceState)
    applyNormalSystemBars()
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

  private fun applyNormalSystemBars() {
    WindowCompat.setDecorFitsSystemWindows(window, true)
    @Suppress("DEPRECATION")
    window.decorView.systemUiVisibility = normalSystemUiVisibilityFlags()
    val controller = WindowCompat.getInsetsController(window, window.decorView)
    controller.isAppearanceLightStatusBars = true
    controller.isAppearanceLightNavigationBars = true
  }

  @Suppress("DEPRECATION")
  private fun normalSystemUiVisibilityFlags(): Int {
    var flags = View.SYSTEM_UI_FLAG_LAYOUT_STABLE or View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
      flags = flags or View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR
    }
    return flags
  }

  private companion object {
    private const val EXIT_CONFIRM_WINDOW_MS = 3_000L
  }
}
