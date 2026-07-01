plugins {
    id("com.android.library")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "vip.baka.bangumipipeline.player"
    compileSdk = 35

    defaultConfig {
        minSdk = 24
    }
}

dependencies {
    implementation("androidx.core:core-ktx:1.13.1")
}
