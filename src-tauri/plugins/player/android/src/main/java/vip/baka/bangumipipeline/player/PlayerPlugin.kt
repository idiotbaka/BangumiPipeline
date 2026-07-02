package vip.baka.bangumipipeline.player

import android.app.Activity
import android.content.pm.ActivityInfo
import android.os.Build
import android.view.View
import android.view.WindowManager
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin

@InvokeArg
class FullscreenArgs {
    var orientation: String? = null
}

@InvokeArg
class KeepScreenOnArgs {
    var enabled: Boolean = false
}

@TauriPlugin
class PlayerPlugin(private val activity: Activity) : Plugin(activity) {
    private var previousOrientation: Int? = null
    private var fullscreenActive = false
    private var playbackKeepScreenOn = false
    private val immersiveRefreshRunnable = Runnable { applyFullscreenImmersive() }

    @Command
    fun enterFullscreen(invoke: Invoke) {
        val args = invoke.parseArgs(FullscreenArgs::class.java)
        activity.runOnUiThread {
            if (previousOrientation == null) {
                previousOrientation = activity.requestedOrientation
            }
            fullscreenActive = true
            activity.requestedOrientation = requestedOrientation(args.orientation)
            applyKeepScreenOn()
            installImmersiveRefreshHooks()
            scheduleFullscreenImmersiveRefresh()
            invoke.resolve(JSObject())
        }
    }

    @Command
    fun exitFullscreen(invoke: Invoke) {
        activity.runOnUiThread {
            fullscreenActive = false
            activity.requestedOrientation = previousOrientation ?: ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
            previousOrientation = null
            removeImmersiveRefreshHooks()
            applyKeepScreenOn()
            restoreNormalSystemBars()
            invoke.resolve(JSObject())
        }
    }

    @Command
    fun setKeepScreenOn(invoke: Invoke) {
        val args = invoke.parseArgs(KeepScreenOnArgs::class.java)
        activity.runOnUiThread {
            playbackKeepScreenOn = args.enabled
            applyKeepScreenOn()
            scheduleFullscreenImmersiveRefresh()
            invoke.resolve(JSObject())
        }
    }

    override fun onResume() {
        super.onResume()
        activity.runOnUiThread {
            scheduleFullscreenImmersiveRefresh()
        }
    }

    override fun onPause() {
        activity.runOnUiThread {
            activity.window.decorView.removeCallbacks(immersiveRefreshRunnable)
        }
        super.onPause()
    }

    @Suppress("DEPRECATION", "OVERRIDE_DEPRECATION")
    override fun onDestroy() {
        activity.runOnUiThread {
            removeImmersiveRefreshHooks()
        }
        super.onDestroy()
    }

    private fun applyKeepScreenOn() {
        if (fullscreenActive || playbackKeepScreenOn) {
            activity.window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        } else {
            activity.window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        }
    }

    private fun installImmersiveRefreshHooks() {
        val decorView = activity.window.decorView
        @Suppress("DEPRECATION")
        decorView.setOnSystemUiVisibilityChangeListener {
            if (fullscreenActive) {
                scheduleFullscreenImmersiveRefresh()
            }
        }
        ViewCompat.setOnApplyWindowInsetsListener(decorView) { view, insets ->
            if (fullscreenActive && insets.isVisible(WindowInsetsCompat.Type.systemBars())) {
                view.post { scheduleFullscreenImmersiveRefresh() }
            }
            insets
        }
    }

    private fun removeImmersiveRefreshHooks() {
        val decorView = activity.window.decorView
        decorView.removeCallbacks(immersiveRefreshRunnable)
        @Suppress("DEPRECATION")
        decorView.setOnSystemUiVisibilityChangeListener(null)
        ViewCompat.setOnApplyWindowInsetsListener(decorView, null)
    }

    private fun scheduleFullscreenImmersiveRefresh() {
        val decorView = activity.window.decorView
        decorView.removeCallbacks(immersiveRefreshRunnable)
        if (!fullscreenActive) {
            return
        }
        applyFullscreenImmersive()
        decorView.postDelayed(immersiveRefreshRunnable, 90L)
        decorView.postDelayed(immersiveRefreshRunnable, 320L)
    }

    private fun applyFullscreenImmersive() {
        if (!fullscreenActive) {
            return
        }
        val window = activity.window
        val decorView = window.decorView
        WindowCompat.setDecorFitsSystemWindows(window, false)
        val controller = WindowCompat.getInsetsController(window, decorView)
        controller.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        controller.hide(WindowInsetsCompat.Type.systemBars())
        @Suppress("DEPRECATION")
        decorView.systemUiVisibility = (
            View.SYSTEM_UI_FLAG_IMMERSIVE_STICKY
                or View.SYSTEM_UI_FLAG_FULLSCREEN
                or View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
                or View.SYSTEM_UI_FLAG_LAYOUT_STABLE
                or View.SYSTEM_UI_FLAG_LAYOUT_FULLSCREEN
                or View.SYSTEM_UI_FLAG_LAYOUT_HIDE_NAVIGATION
            )
    }

    private fun restoreNormalSystemBars() {
        val window = activity.window
        val decorView = window.decorView
        WindowCompat.setDecorFitsSystemWindows(window, true)
        @Suppress("DEPRECATION")
        decorView.systemUiVisibility = normalSystemUiVisibilityFlags()
        val controller = WindowCompat.getInsetsController(window, decorView)
        controller.show(WindowInsetsCompat.Type.systemBars())
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

    private fun requestedOrientation(value: String?): Int {
        return when (value?.trim()?.lowercase()) {
            "landscape" -> ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
            "reverselandscape", "reverse_landscape" -> ActivityInfo.SCREEN_ORIENTATION_REVERSE_LANDSCAPE
            "fullsensor", "full_sensor" -> ActivityInfo.SCREEN_ORIENTATION_FULL_SENSOR
            "sensor" -> ActivityInfo.SCREEN_ORIENTATION_SENSOR
            "sensorlandscape", "sensor_landscape" -> ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
            else -> ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
        }
    }
}
