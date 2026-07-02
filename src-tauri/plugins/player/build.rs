const COMMANDS: &[&str] = &[
    "enterFullscreen",
    "exitFullscreen",
    "setKeepScreenOn",
    "enter_fullscreen",
    "exit_fullscreen",
    "set_keep_screen_on",
];

fn main() {
    tauri_plugin::Builder::new(COMMANDS)
        .android_path("android")
        .build();
}
