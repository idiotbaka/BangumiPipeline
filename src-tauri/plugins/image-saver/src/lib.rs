use serde::{Deserialize, Serialize};
use tauri::{
    plugin::{Builder, TauriPlugin},
    Runtime,
};

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PrepareImageArgs {
    pub url: String,
    pub headers: Option<std::collections::HashMap<String, String>>,
    pub suggested_name: Option<String>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CachedImageArgs {
    pub cache_key: String,
    pub suggested_name: Option<String>,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct PreparedImage {
    pub cache_key: String,
    pub byte_size: u64,
    pub mime_type: String,
    pub suggested_name: String,
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SavedImage {
    pub display_name: String,
    pub path: String,
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("image-saver")
        .setup(|_app, _api| {
            #[cfg(target_os = "android")]
            _api.register_android_plugin(
                "vip.baka.bangumipipeline.imagesaver",
                "ImageSaverPlugin",
            )?;
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            prepare_image,
            save_image,
            discard_image
        ])
        .build()
}

#[tauri::command]
async fn prepare_image(_args: Option<PrepareImageArgs>) -> Result<PreparedImage, String> {
    Err("原生图片保存仅在 Android APP 中可用".to_string())
}

#[tauri::command]
async fn save_image(_args: Option<CachedImageArgs>) -> Result<SavedImage, String> {
    Err("原生图片保存仅在 Android APP 中可用".to_string())
}

#[tauri::command]
async fn discard_image(_args: Option<CachedImageArgs>) -> Result<(), String> {
    Ok(())
}
