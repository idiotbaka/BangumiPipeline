package vip.baka.bangumipipeline.player

import android.app.Activity
import android.content.Context
import android.graphics.Color
import android.graphics.drawable.ColorDrawable
import android.os.Build
import android.view.View
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat

/** WebView 主题的原生镜像，仅用于系统栏和 WebView 创建前的窗口背景。 */
object AppTheme {
    private const val PREFERENCES = "bp.mobile.appearance"
    private const val DARK_MODE = "dark"

    fun isDark(context: Context): Boolean =
        context.getSharedPreferences(PREFERENCES, Context.MODE_PRIVATE).getBoolean(DARK_MODE, false)

    fun save(context: Context, dark: Boolean) {
        context.getSharedPreferences(PREFERENCES, Context.MODE_PRIVATE).edit().putBoolean(DARK_MODE, dark).apply()
    }

    fun backgroundColor(context: Context): Int =
        if (isDark(context)) Color.rgb(17, 21, 31) else Color.WHITE

    @Suppress("DEPRECATION")
    fun applySystemBars(activity: Activity) {
        val dark = isDark(activity)
        val window = activity.window
        val background = backgroundColor(activity)
        window.setBackgroundDrawable(ColorDrawable(background))
        window.statusBarColor = background
        // Android 7 不支持深色导航栏图标，浅色模式下也用深底承托系统白色图标。
        window.navigationBarColor = if (!dark && Build.VERSION.SDK_INT < Build.VERSION_CODES.O) {
            Color.rgb(32, 40, 62)
        } else background
        WindowCompat.setDecorFitsSystemWindows(window, true)
        var flags = View.SYSTEM_UI_FLAG_LAYOUT_STABLE
        if (!dark) {
            flags = flags or View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                flags = flags or View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR
            }
        }
        window.decorView.systemUiVisibility = flags
        val controller = WindowCompat.getInsetsController(window, window.decorView)
        controller.isAppearanceLightStatusBars = !dark
        controller.isAppearanceLightNavigationBars = !dark
        controller.show(WindowInsetsCompat.Type.systemBars())
    }
}
