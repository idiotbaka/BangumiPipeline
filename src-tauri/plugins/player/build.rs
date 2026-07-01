const COMMANDS: &[&str] = &[
    "enterFullscreen",
    "exitFullscreen",
    "enter_fullscreen",
    "exit_fullscreen",
];

fn main() {
    tauri_plugin::Builder::new(COMMANDS).build();
}
