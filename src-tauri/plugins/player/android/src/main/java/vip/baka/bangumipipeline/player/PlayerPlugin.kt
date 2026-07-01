package vip.baka.bangumipipeline.player

import android.app.Activity
import android.content.pm.ActivityInfo
import android.view.WindowManager
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

@TauriPlugin
class PlayerPlugin(private val activity: Activity) : Plugin(activity) {
    private var previousOrientation: Int? = null

    @Command
    fun enterFullscreen(invoke: Invoke) {
        val args = invoke.parseArgs(FullscreenArgs::class.java)
        activity.runOnUiThread {
            if (previousOrientation == null) {
                previousOrientation = activity.requestedOrientation
            }
            activity.requestedOrientation = requestedOrientation(args.orientation)
            activity.window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
            val controller = WindowCompat.getInsetsController(activity.window, activity.window.decorView)
            controller.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            controller.hide(WindowInsetsCompat.Type.systemBars())
            invoke.resolve(JSObject())
        }
    }

    @Command
    fun exitFullscreen(invoke: Invoke) {
        activity.runOnUiThread {
            activity.requestedOrientation = previousOrientation ?: ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
            previousOrientation = null
            activity.window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
            WindowCompat.getInsetsController(activity.window, activity.window.decorView)
                .show(WindowInsetsCompat.Type.systemBars())
            invoke.resolve(JSObject())
        }
    }

    private fun requestedOrientation(value: String?): Int {
        return when (value?.trim()?.lowercase()) {
            "landscape" -> ActivityInfo.SCREEN_ORIENTATION_LANDSCAPE
            "reverseLandscape" -> ActivityInfo.SCREEN_ORIENTATION_REVERSE_LANDSCAPE
            "fullSensor" -> ActivityInfo.SCREEN_ORIENTATION_FULL_SENSOR
            "sensor" -> ActivityInfo.SCREEN_ORIENTATION_SENSOR
            else -> ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
        }
    }
}
