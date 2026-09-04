use std::path::Path;

use crate::paths::{ensure_file_mode, ensure_parent_mode, RUNTIME_DIRECTORY_MODE, RUNTIME_METADATA_MODE};
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct Config {
    pub default_workspace: String,
}

#[derive(Debug)]
pub enum ConfigError {
    Io(std::io::Error),
}

impl std::fmt::Display for ConfigError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Io(source) => write!(f, "{source}"),
        }
    }
}

impl std::error::Error for ConfigError {}

pub fn read_config(path: &Path) -> Result<Config, ConfigError> {
    let contents = match std::fs::read_to_string(path) {
        Ok(text) => text,
        Err(source) if source.kind() == std::io::ErrorKind::NotFound => return Ok(Config::default()),
        Err(source) => return Err(ConfigError::Io(source)),
    };
    let mut config = Config::default();
    for line in contents.lines() {
        let Some((key, value)) = line.split_once(':') else { continue };
        if key.trim() != "default_workspace" {
            continue;
        }
        config.default_workspace = value.trim().trim_matches(|c| c == '"' || c == '\'').to_string();
    }
    Ok(config)
}

pub fn write_config(path: &Path, config: &Config) -> Result<(), ConfigError> {
    ensure_parent_mode(path, RUNTIME_DIRECTORY_MODE).map_err(ConfigError::Io)?;
    let mut contents = String::from("runtime_version: go-v1\n");
    if !config.default_workspace.is_empty() {
        contents.push_str("default_workspace: ");
        contents.push_str(&config.default_workspace);
        contents.push('\n');
    }
    write_file_mode(path, contents.as_bytes()).map_err(ConfigError::Io)
}

fn write_file_mode(path: &Path, contents: &[u8]) -> Result<(), std::io::Error> {
    std::fs::write(path, contents)?;
    ensure_file_mode(path, RUNTIME_METADATA_MODE)
}

pub fn ensure_config(path: &Path) -> Result<bool, ConfigError> {
    match std::fs::metadata(path) {
        Ok(_) => {
            ensure_parent_mode(path, RUNTIME_DIRECTORY_MODE).map_err(ConfigError::Io)?;
            ensure_file_mode(path, RUNTIME_METADATA_MODE).map_err(ConfigError::Io)?;
            Ok(false)
        }
        Err(source) if source.kind() == std::io::ErrorKind::NotFound => {
            write_config(path, &Config::default()).map(|_| true)
        }
        Err(source) => Err(ConfigError::Io(source)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_file(name: &str) -> (std::path::PathBuf, std::path::PathBuf) {
        let dir = std::env::temp_dir().join(format!("zbrain-config-{}-{name}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        std::fs::create_dir_all(&dir).unwrap();
        let path = dir.join(name);
        (dir, path)
    }

    #[test]
    fn read_missing_config_is_empty() {
        let (dir, path) = temp_file("missing.yml");
        let config = read_config(&path).unwrap();
        assert_eq!(config, Config::default());
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn write_then_read_round_trip() {
        let (dir, path) = temp_file("round.yml");
        write_config(&path, &Config { default_workspace: "research".into() }).unwrap();
        let config = read_config(&path).unwrap();
        assert_eq!(config.default_workspace, "research");
        use std::os::unix::fs::PermissionsExt;
        let mode = std::fs::metadata(&path).unwrap().permissions().mode();
        assert_eq!(mode & 0o777, 0o600);
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn ensure_config_creates_once() {
        let (dir, path) = temp_file("ensure.yml");
        assert!(ensure_config(&path).unwrap());
        assert!(!ensure_config(&path).unwrap());
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn quoted_values_are_trimmed() {
        let (dir, path) = temp_file("quoted.yml");
        std::fs::write(&path, "default_workspace: \"research\"\n").unwrap();
        let config = read_config(&path).unwrap();
        assert_eq!(config.default_workspace, "research");
        let _ = std::fs::remove_dir_all(&dir);
    }
}
