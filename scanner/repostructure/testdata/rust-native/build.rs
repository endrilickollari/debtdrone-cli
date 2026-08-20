fn main() {
    cmake::build("native");
    bindgen::Builder::default();
    prost_build::compile_protos(&["schema.proto"], &["."]);
    pkg_config::probe_library("ssl");
}
