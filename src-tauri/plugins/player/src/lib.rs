use serde::Deserialize;
use tauri::{
    plugin::{Builder, TauriPlugin},
    Runtime,
};

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct FullscreenArgs {
    pub orientation: Option<String>,
}

pub fn init<R: Runtime>() -> TauriPlugin<R> {
    Builder::new("player")
        .setup(|_app, _api| {
            #[cfg(target_os = "android")]
            _api.register_android_plugin("vip.baka.bangumipipeline.player", "PlayerPlugin")?;
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![enter_fullscreen, exit_fullscreen])
        .build()
}

#[tauri::command]
async fn enter_fullscreen(_args: Option<FullscreenArgs>) -> Result<(), String> {
    Ok(())
}

#[tauri::command]
async fn exit_fullscreen() -> Result<(), String> {
    Ok(())
}
