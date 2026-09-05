#[test]
fn bundled_sqlite_compiles_and_runs_fts5() {
    use rusqlite::Connection;
    let conn = Connection::open_in_memory().expect("open in-memory db");
    let enabled: i64 = conn
        .query_row("select sqlite_compileoption_used('ENABLE_FTS5')", [], |row| {
            row.get(0)
        })
        .expect("compileoption query");
    assert_eq!(enabled, 1, "FTS5 compile option missing in bundled build");
    conn.execute("create virtual table fts_probe using fts5(body)", [])
        .expect("create FTS5 table");
    conn.execute("insert into fts_probe(body) values ('trusted memory retrieval')", [])
        .expect("insert");
    let hits: i64 = conn
        .query_row(
            "select count(*) from fts_probe where fts_probe match 'trust*'",
            [],
            |row| row.get(0),
        )
        .expect("fts match query");
    assert_eq!(hits, 1, "prefix match should hit");
    let version: String = conn
        .query_row("select sqlite_version()", [], |row| row.get(0))
        .expect("sqlite version");
    println!("sqlite {version} with FTS5 enabled");
}
