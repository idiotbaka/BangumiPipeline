package vip.baka.bangumipipeline.mobile

import android.os.Bundle
import android.os.SystemClock
import android.view.KeyEvent
import android.webkit.WebView
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat

class MainActivity : TauriActivity() {
  override val handleBackNavigation: Boolean = false

  private var appWebView: WebView? = null
  private var lastBackPressedAt = 0L
  private var exitToast: Toast? = null

  override fun onCreate(savedInstanceState: Bundle?) {
    super.onCreate(savedInstanceState)
    applyTVSystemBars()
  }

  override fun onWindowFocusChanged(hasFocus: Boolean) {
    super.onWindowFocusChanged(hasFocus)
    if (hasFocus) {
      applyTVSystemBars()
    }
  }

  override fun onWebViewCreate(webView: WebView) {
    super.onWebViewCreate(webView)
    appWebView = webView
    webView.settings.mediaPlaybackRequiresUserGesture = false
    webView.isFocusable = true
    webView.isFocusableInTouchMode = true
    webView.requestFocus()

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
            "再按一次退出 BakaVip2 TV",
            Toast.LENGTH_SHORT,
          )
          exitToast?.show()
        }
      },
    )
  }

  override fun dispatchKeyEvent(event: KeyEvent): Boolean {
    if (event.action == KeyEvent.ACTION_DOWN) {
      val remoteKey = when (event.keyCode) {
        KeyEvent.KEYCODE_BUTTON_A -> "select"
        KeyEvent.KEYCODE_MEDIA_PLAY_PAUSE -> "playPause"
        KeyEvent.KEYCODE_MEDIA_PLAY -> "play"
        KeyEvent.KEYCODE_MEDIA_PAUSE -> "pause"
        KeyEvent.KEYCODE_MEDIA_REWIND -> "seekBackward"
        KeyEvent.KEYCODE_MEDIA_FAST_FORWARD -> "seekForward"
        else -> null
      }
      if (remoteKey != null && (event.repeatCount == 0 || remoteKey.startsWith("seek"))) {
        dispatchTVKey(remoteKey)
        return true
      }
    }
    return super.dispatchKeyEvent(event)
  }

  override fun onDestroy() {
    appWebView = null
    exitToast?.cancel()
    exitToast = null
    super.onDestroy()
  }

  private fun dispatchTVKey(key: String) {
    val script =
      "window.dispatchEvent(new CustomEvent('bp-tv-key',{detail:{key:'$key'}}));"
    appWebView?.post {
      appWebView?.evaluateJavascript(script, null)
    }
  }

  @Suppress("DEPRECATION")
  private fun applyTVSystemBars() {
    val decorView = window.decorView
    WindowCompat.setDecorFitsSystemWindows(window, false)
    val controller = WindowCompat.getInsetsController(window, decorView)
    controller.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
    controller.hide(WindowInsetsCompat.Type.systemBars())
    decorView.systemUiVisibility = (
      android.view.View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
        or android.view.View.SYSTEM_UI_FLAG_FULLSCREEN
        or android.view.View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
        or android.view.View.SYSTEM_UI_FLAG_LAYOUT_STABLE
        or android.view.View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
        or android.view.View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
      )
  }

  private companion object {
    private const val EXIT_CONFIRM_WINDOW_MS = 3_000L
  }
}
