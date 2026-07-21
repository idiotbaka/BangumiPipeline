package vip.baka.bangumipipeline.imagesaver

import android.Manifest
import android.app.Activity
import android.content.ContentValues
import android.media.MediaScannerConnection
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import app.tauri.PermissionState
import app.tauri.annotation.Command
import app.tauri.annotation.InvokeArg
import app.tauri.annotation.Permission
import app.tauri.annotation.PermissionCallback
import app.tauri.annotation.TauriPlugin
import app.tauri.plugin.Invoke
import app.tauri.plugin.JSObject
import app.tauri.plugin.Plugin
import java.io.BufferedInputStream
import java.io.File
import java.io.FileInputStream
import java.io.FileOutputStream
import java.net.HttpURLConnection
import java.net.URL
import java.nio.charset.StandardCharsets
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import java.util.UUID
import java.util.concurrent.Executors

private const val PUBLIC_IMAGES_PERMISSION = "publicImages"

@InvokeArg
class PrepareImageArgs {
    var url: String = ""
    var headers: HashMap<String, String> = hashMapOf()
    var suggestedName: String? = null
}

@InvokeArg
class CachedImageArgs {
    var cacheKey: String = ""
    var suggestedName: String? = null
}

private data class DownloadResult(
    val mimeHint: String,
    val finalUrl: URL,
    val byteSize: Long,
)

private data class ImageFormat(
    val mimeType: String,
    val extension: String,
)

@TauriPlugin(
    permissions = [
        Permission(
            strings = [Manifest.permission.WRITE_EXTERNAL_STORAGE],
            alias = PUBLIC_IMAGES_PERMISSION,
        ),
    ],
)
class ImageSaverPlugin(private val activity: Activity) : Plugin(activity) {
    private val executor = Executors.newSingleThreadExecutor()

    @Command
    fun prepareImage(invoke: Invoke) {
        val args = invoke.parseArgs(PrepareImageArgs::class.java)
        executor.execute {
            try {
                resolve(invoke, prepareImage(args))
            } catch (error: Exception) {
                reject(invoke, error.message ?: "读取原图失败")
            }
        }
    }

    @Command
    fun saveImage(invoke: Invoke) {
        if (Build.VERSION.SDK_INT <= Build.VERSION_CODES.P &&
            getPermissionState(PUBLIC_IMAGES_PERMISSION) != PermissionState.GRANTED
        ) {
            requestPermissionForAlias(PUBLIC_IMAGES_PERMISSION, invoke, "saveImageAfterPermission")
            return
        }
        saveImageInBackground(invoke)
    }

    @PermissionCallback
    fun saveImageAfterPermission(invoke: Invoke) {
        if (getPermissionState(PUBLIC_IMAGES_PERMISSION) != PermissionState.GRANTED) {
            invoke.reject("需要存储权限才能把图片保存到公共 Pictures 文件夹")
            return
        }
        saveImageInBackground(invoke)
    }

    @Command
    fun discardImage(invoke: Invoke) {
        val args = invoke.parseArgs(CachedImageArgs::class.java)
        executor.execute {
            try {
                cachedImageFile(args.cacheKey).delete()
                resolve(invoke, JSObject())
            } catch (error: Exception) {
                reject(invoke, error.message ?: "清理图片缓存失败")
            }
        }
    }

    @Suppress("OVERRIDE_DEPRECATION")
    override fun onDestroy() {
        executor.shutdownNow()
        super.onDestroy()
    }

    private fun saveImageInBackground(invoke: Invoke) {
        val args = invoke.parseArgs(CachedImageArgs::class.java)
        executor.execute {
            try {
                resolve(invoke, saveImage(args))
            } catch (error: Exception) {
                reject(invoke, error.message ?: "保存原图失败")
            }
        }
    }

    private fun prepareImage(args: PrepareImageArgs): JSObject {
        val sourceUrl = validatedHttpUrl(args.url)
        cleanupExpiredCache()
        val directory = imageCacheDirectory()
        val pending = File.createTempFile("image-", ".part", directory)
        try {
            val download = downloadImage(sourceUrl, args.headers, pending)
            val format = detectImageFormat(pending)
                ?: throw IllegalArgumentException("该地址返回的内容不是支持的图片格式")
            val cacheKey = "${UUID.randomUUID()}.${format.extension}"
            val cached = File(directory, cacheKey)
            if (!pending.renameTo(cached)) {
                pending.copyTo(cached, overwrite = true)
                pending.delete()
            }
            val suggestedName = normalizedSuggestedName(args.suggestedName, download.finalUrl, format.extension)
            return JSObject().apply {
                put("cacheKey", cacheKey)
                put("byteSize", download.byteSize)
                put("mimeType", format.mimeType.ifBlank { download.mimeHint })
                put("suggestedName", suggestedName)
            }
        } catch (error: Exception) {
            pending.delete()
            throw error
        }
    }

    private fun downloadImage(
        sourceUrl: URL,
        headers: Map<String, String>,
        target: File,
    ): DownloadResult {
        var currentUrl = sourceUrl
        var redirects = 0
        while (true) {
            val connection = currentUrl.openConnection() as? HttpURLConnection
                ?: throw IllegalArgumentException("只支持 HTTP 或 HTTPS 图片")
            connection.instanceFollowRedirects = false
            connection.connectTimeout = CONNECT_TIMEOUT_MS
            connection.readTimeout = READ_TIMEOUT_MS
            connection.setRequestProperty("Accept", "image/*,*/*;q=0.5")
            connection.setRequestProperty("User-Agent", "BakaVip2/1.1 Android")
            if (sameOrigin(sourceUrl, currentUrl)) {
                applySafeHeaders(connection, headers)
            }
            try {
                val status = connection.responseCode
                if (status in REDIRECT_STATUS_CODES) {
                    val location = connection.getHeaderField("Location")
                        ?: throw IllegalStateException("图片地址重定向缺少目标")
                    if (++redirects > MAX_REDIRECTS) {
                        throw IllegalStateException("图片地址重定向次数过多")
                    }
                    currentUrl = validatedHttpUrl(URL(currentUrl, location).toString())
                    continue
                }
                if (status !in 200..299) {
                    throw IllegalStateException("读取原图失败（HTTP $status）")
                }
                val expectedSize = connection.contentLengthLong
                if (expectedSize > MAX_IMAGE_BYTES) {
                    throw IllegalStateException("原图超过 50 MB，无法保存")
                }
                var total = 0L
                BufferedInputStream(connection.inputStream).use { input ->
                    FileOutputStream(target).use { output ->
                        val buffer = ByteArray(COPY_BUFFER_SIZE)
                        while (true) {
                            val count = input.read(buffer)
                            if (count < 0) break
                            total += count
                            if (total > MAX_IMAGE_BYTES) {
                                throw IllegalStateException("原图超过 50 MB，无法保存")
                            }
                            output.write(buffer, 0, count)
                        }
                    }
                }
                if (total <= 0L) throw IllegalStateException("原图内容为空")
                return DownloadResult(
                    mimeHint = connection.contentType?.substringBefore(';')?.trim().orEmpty(),
                    finalUrl = currentUrl,
                    byteSize = total,
                )
            } finally {
                connection.disconnect()
            }
        }
    }

    private fun saveImage(args: CachedImageArgs): JSObject {
        val source = cachedImageFile(args.cacheKey)
        if (!source.isFile) throw IllegalStateException("原图缓存已失效，请重新打开图片")
        val format = detectImageFormat(source)
            ?: throw IllegalArgumentException("图片缓存格式无效")
        val displayName = savedDisplayName(args.suggestedName, format.extension)
        val savedPath = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            saveWithMediaStore(source, displayName, format.mimeType)
        } else {
            saveToLegacyPictures(source, displayName, format.mimeType)
        }
        return JSObject().apply {
            put("displayName", displayName)
            put("path", savedPath)
        }
    }

    private fun saveWithMediaStore(source: File, displayName: String, mimeType: String): String {
        val resolver = activity.contentResolver
        val relativeDirectory = "${Environment.DIRECTORY_PICTURES}/$PUBLIC_FOLDER_NAME"
        val values = ContentValues().apply {
            put(MediaStore.Images.Media.DISPLAY_NAME, displayName)
            put(MediaStore.Images.Media.MIME_TYPE, mimeType)
            put(MediaStore.Images.Media.RELATIVE_PATH, relativeDirectory)
            put(MediaStore.Images.Media.IS_PENDING, 1)
        }
        val collection = MediaStore.Images.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
        val uri = resolver.insert(collection, values)
            ?: throw IllegalStateException("无法创建公共图片文件")
        try {
            resolver.openOutputStream(uri, "w")?.use { output ->
                FileInputStream(source).use { input -> input.copyTo(output, COPY_BUFFER_SIZE) }
            } ?: throw IllegalStateException("无法写入公共图片文件")
            values.clear()
            values.put(MediaStore.Images.Media.IS_PENDING, 0)
            resolver.update(uri, values, null, null)
        } catch (error: Exception) {
            resolver.delete(uri, null, null)
            throw error
        }
        return publicPicturesPath(displayName)
    }

    @Suppress("DEPRECATION")
    private fun saveToLegacyPictures(source: File, displayName: String, mimeType: String): String {
        val directory = File(
            Environment.getExternalStoragePublicDirectory(Environment.DIRECTORY_PICTURES),
            PUBLIC_FOLDER_NAME,
        )
        if (!directory.exists() && !directory.mkdirs()) {
            throw IllegalStateException("无法创建 Pictures/$PUBLIC_FOLDER_NAME 文件夹")
        }
        val target = uniqueLegacyFile(directory, displayName)
        FileInputStream(source).use { input ->
            FileOutputStream(target).use { output -> input.copyTo(output, COPY_BUFFER_SIZE) }
        }
        MediaScannerConnection.scanFile(activity, arrayOf(target.absolutePath), arrayOf(mimeType), null)
        return target.absolutePath
    }

    private fun cachedImageFile(cacheKey: String): File {
        if (!CACHE_KEY_PATTERN.matches(cacheKey)) {
            throw IllegalArgumentException("图片缓存标识无效")
        }
        val directory = imageCacheDirectory().canonicalFile
        val file = File(directory, cacheKey).canonicalFile
        if (file.parentFile != directory) throw IllegalArgumentException("图片缓存路径无效")
        return file
    }

    private fun imageCacheDirectory(): File {
        val directory = File(activity.cacheDir, CACHE_DIRECTORY_NAME)
        if (!directory.exists() && !directory.mkdirs()) {
            throw IllegalStateException("无法创建图片缓存目录")
        }
        return directory
    }

    private fun cleanupExpiredCache() {
        val cutoff = System.currentTimeMillis() - CACHE_MAX_AGE_MS
        imageCacheDirectory().listFiles()?.forEach { file ->
            if (file.isFile && file.lastModified() < cutoff) file.delete()
        }
    }

    private fun validatedHttpUrl(value: String): URL {
        val url = URL(value.trim())
        if (url.protocol.lowercase(Locale.US) !in setOf("http", "https") || url.host.isBlank()) {
            throw IllegalArgumentException("图片地址无效")
        }
        if (url.userInfo != null) throw IllegalArgumentException("图片地址不能包含账号信息")
        return url
    }

    private fun sameOrigin(first: URL, second: URL): Boolean {
        return first.protocol.equals(second.protocol, true) &&
            first.host.equals(second.host, true) &&
            effectivePort(first) == effectivePort(second)
    }

    private fun effectivePort(url: URL): Int {
        return if (url.port >= 0) url.port else url.defaultPort
    }

    private fun applySafeHeaders(connection: HttpURLConnection, headers: Map<String, String>) {
        headers.forEach { (name, value) ->
            val normalized = name.trim().lowercase(Locale.US)
            if (normalized !in BLOCKED_REQUEST_HEADERS &&
                HEADER_NAME_PATTERN.matches(name.trim()) &&
                !value.contains('\r') &&
                !value.contains('\n')
            ) {
                connection.setRequestProperty(name.trim(), value)
            }
        }
    }

    private fun detectImageFormat(file: File): ImageFormat? {
        val prefix = ByteArray(1024)
        val count = FileInputStream(file).use { it.read(prefix) }
        if (count <= 0) return null
        fun byte(index: Int) = if (index < count) prefix[index].toInt() and 0xff else -1
        fun ascii(start: Int, length: Int): String {
            if (start < 0 || start + length > count) return ""
            return String(prefix, start, length, StandardCharsets.US_ASCII)
        }
        if (byte(0) == 0xff && byte(1) == 0xd8 && byte(2) == 0xff) {
            return ImageFormat("image/jpeg", "jpg")
        }
        if (byte(0) == 0x89 && ascii(1, 3) == "PNG") {
            return ImageFormat("image/png", "png")
        }
        if (ascii(0, 6) == "GIF87a" || ascii(0, 6) == "GIF89a") {
            return ImageFormat("image/gif", "gif")
        }
        if (ascii(0, 4) == "RIFF" && ascii(8, 4) == "WEBP") {
            return ImageFormat("image/webp", "webp")
        }
        if (ascii(0, 2) == "BM") return ImageFormat("image/bmp", "bmp")
        if (ascii(4, 4) == "ftyp") {
            val brand = ascii(8, 4).lowercase(Locale.US)
            if (brand == "avif" || brand == "avis") return ImageFormat("image/avif", "avif")
            if (brand in setOf("heic", "heix", "hevc", "hevx", "mif1")) {
                return ImageFormat("image/heic", "heic")
            }
        }
        val text = String(prefix, 0, count, StandardCharsets.UTF_8).trimStart().lowercase(Locale.US)
        if (text.startsWith("<svg") || (text.startsWith("<?xml") && text.contains("<svg"))) {
            return ImageFormat("image/svg+xml", "svg")
        }
        return null
    }

    private fun normalizedSuggestedName(value: String?, sourceUrl: URL, extension: String): String {
        val requested = value?.trim().orEmpty()
        val sourceName = sourceUrl.path.substringAfterLast('/').substringBefore('?')
        val base = sanitizeFileBase(requested.ifBlank { sourceName }).ifBlank { "bakavip2-image" }
        return "${base.substringBeforeLast('.', base).take(MAX_FILE_BASE_LENGTH)}.$extension"
    }

    private fun savedDisplayName(value: String?, extension: String): String {
        val sanitized = sanitizeFileBase(value.orEmpty())
        val base = sanitized.substringBeforeLast('.', sanitized)
            .ifBlank { "bakavip2-image" }
            .take(MAX_FILE_BASE_LENGTH)
        val timestamp = SimpleDateFormat("yyyyMMdd-HHmmss-SSS", Locale.US).format(Date())
        return "$base-$timestamp.$extension"
    }

    private fun sanitizeFileBase(value: String): String {
        return value.replace(INVALID_FILE_CHARS, "_").trim().trim('.').trim()
    }

    private fun uniqueLegacyFile(directory: File, displayName: String): File {
        var target = File(directory, displayName)
        if (!target.exists()) return target
        val extension = displayName.substringAfterLast('.', "jpg")
        val base = displayName.substringBeforeLast('.', displayName)
        var index = 2
        while (target.exists()) {
            target = File(directory, "$base-$index.$extension")
            index++
        }
        return target
    }

    @Suppress("DEPRECATION")
    private fun publicPicturesPath(displayName: String): String {
        return File(
            File(Environment.getExternalStorageDirectory(), Environment.DIRECTORY_PICTURES),
            "$PUBLIC_FOLDER_NAME/$displayName",
        ).absolutePath
    }

    private fun resolve(invoke: Invoke, value: JSObject) {
        activity.runOnUiThread { invoke.resolve(value) }
    }

    private fun reject(invoke: Invoke, message: String) {
        activity.runOnUiThread { invoke.reject(message) }
    }

    private companion object {
        private const val PUBLIC_FOLDER_NAME = "bakavip2"
        private const val CACHE_DIRECTORY_NAME = "prepared-images"
        private const val MAX_IMAGE_BYTES = 50L * 1024L * 1024L
        private const val CACHE_MAX_AGE_MS = 24L * 60L * 60L * 1000L
        private const val CONNECT_TIMEOUT_MS = 20_000
        private const val READ_TIMEOUT_MS = 60_000
        private const val COPY_BUFFER_SIZE = 64 * 1024
        private const val MAX_REDIRECTS = 5
        private const val MAX_FILE_BASE_LENGTH = 80
        private val REDIRECT_STATUS_CODES = setOf(301, 302, 303, 307, 308)
        private val CACHE_KEY_PATTERN = Regex("^[0-9a-f-]{36}\\.(jpg|png|gif|webp|bmp|svg|avif|heic)$")
        private val HEADER_NAME_PATTERN = Regex("^[A-Za-z0-9!#$%&'*+.^_`|~-]+$")
        private val INVALID_FILE_CHARS = Regex("[\\p{Cntrl}/\\\\:*?\"<>|]+")
        private val BLOCKED_REQUEST_HEADERS = setOf(
            "connection",
            "content-length",
            "host",
            "proxy-authorization",
            "transfer-encoding",
        )
    }
}
