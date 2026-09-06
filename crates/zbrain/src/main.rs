fn main() {
    let paths = match zbrain::paths::Paths::resolve(zbrain::paths::Options::default()) {
        Ok(paths) => paths,
        Err(err) => {
            eprintln!("{err}");
            std::process::exit(1);
        }
    };
    let mut app = zbrain::cli::new_app(paths);
    let args: Vec<String> = std::env::args().skip(1).collect();
    if let Err(err) = app.run(&args) {
        eprintln!("{err}");
        std::process::exit(err.exit_code());
    }
}
