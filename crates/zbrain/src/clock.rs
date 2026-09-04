use std::fmt;

use chrono::{DateTime, Utc};

pub trait Clock: Send + Sync {
    fn now(&self) -> DateTime<Utc>;
}

#[derive(Debug, Clone, Copy)]
pub struct SystemClock;

impl Clock for SystemClock {
    fn now(&self) -> DateTime<Utc> {
        Utc::now()
    }
}

#[derive(Debug)]
pub struct FixedClock(DateTime<Utc>);

impl FixedClock {
    pub fn new(at: DateTime<Utc>) -> Self {
        Self(at)
    }

    pub fn set(&mut self, at: DateTime<Utc>) {
        self.0 = at;
    }
}

impl Clock for FixedClock {
    fn now(&self) -> DateTime<Utc> {
        self.0
    }
}

pub fn rfc3339(at: DateTime<Utc>) -> String {
    at.to_rfc3339_opts(chrono::SecondsFormat::Secs, true)
}

pub fn parse_rfc3339(value: &str) -> Result<DateTime<Utc>, ParseTimestampError> {
    DateTime::parse_from_rfc3339(value)
        .map(|dt| dt.with_timezone(&Utc))
        .map_err(|source| ParseTimestampError {
            value: value.to_string(),
            source,
        })
}

#[derive(Debug)]
pub struct ParseTimestampError {
    value: String,
    source: chrono::ParseError,
}

impl fmt::Display for ParseTimestampError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "invalid rfc3339 timestamp {:?}: {}", self.value, self.source)
    }
}

impl std::error::Error for ParseTimestampError {}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::TimeZone;

    #[test]
    fn fixed_clock_returns_injected_time() {
        let at = Utc.with_ymd_and_hms(2026, 7, 30, 10, 0, 0).unwrap();
        let mut clock = FixedClock::new(at);
        assert_eq!(clock.now(), at);
        let later = at + chrono::Duration::hours(1);
        clock.set(later);
        assert_eq!(clock.now(), later);
    }

    #[test]
    fn rfc3339_round_trip() {
        let at = Utc.with_ymd_and_hms(2026, 7, 30, 10, 0, 0).unwrap();
        let text = rfc3339(at);
        assert_eq!(parse_rfc3339(&text).unwrap(), at);
    }

    #[test]
    fn rfc3339_rejects_garbage() {
        assert!(parse_rfc3339("not-a-time").is_err());
    }
}
