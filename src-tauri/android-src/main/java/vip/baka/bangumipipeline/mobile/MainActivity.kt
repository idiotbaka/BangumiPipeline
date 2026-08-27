package vip.baka.bangumipipeline.mobile

import android.os.Bundle
import android.os.SystemClock
import android.view.WindowManager
import android.webkit.WebView
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import vip.baka.bangumipipeline.player.AppTheme

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
    webView.setBackgroundColor(AppTheme.backgroundColor(this))

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

  @Suppress("DEPRECATION")
  private fun applyNormalSystemBars() {
    window.setSoftInputMode(WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE)
    AppTheme.applySystemBars(this)
  }

  private companion object {
    private const val EXIT_CONFIRM_WINDOW_MS = 3_000L
  }
}
