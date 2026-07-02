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

@InvokeArg
class KeepScreenOnArgs {
    var enabled: Boolean = false
}

@TauriPlugin
class PlayerPlugin(private val activity: Activity) : Plugin(activity) {
    private var previousOrientation: Int? = null
    private var fullscreenActive = false
    private var playbackKeepScreenOn = false

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
            val controller = WindowCompat.getInsetsController(activity.window, activity.window.decorView)
            controller.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            controller.hide(WindowInsetsCompat.Type.systemBars())
            invoke.resolve(JSObject())
        }
    }

    @Command
    fun exitFullscreen(invoke: Invoke) {
        activity.runOnUiThread {
            fullscreenActive = false
            activity.requestedOrientation = previousOrientation ?: ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
            previousOrientation = null
            applyKeepScreenOn()
            WindowCompat.getInsetsController(activity.window, activity.window.decorView)
                .show(WindowInsetsCompat.Type.systemBars())
            invoke.resolve(JSObject())
        }
    }

    @Command
    fun setKeepScreenOn(invoke: Invoke) {
        val args = invoke.parseArgs(KeepScreenOnArgs::class.java)
        activity.runOnUiThread {
            playbackKeepScreenOn = args.enabled
            applyKeepScreenOn()
            invoke.resolve(JSObject())
        }
    }

    private fun applyKeepScreenOn() {
        if (fullscreenActive || playbackKeepScreenOn) {
            activity.window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        } else {
            activity.window.clearFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        }
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
