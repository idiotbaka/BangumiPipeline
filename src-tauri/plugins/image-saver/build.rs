const COMMANDS: &[&str] = &[
    "prepareImage",
    "saveImage",
    "discardImage",
    "prepare_image",
    "save_image",
    "discard_image",
];

fn main() {
    tauri_plugin::Builder::new(COMMANDS)
        .android_path("android")
        .build();
}
