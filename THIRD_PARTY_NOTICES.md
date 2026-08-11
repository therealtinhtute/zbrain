# Third-Party Notices

zbrain (MIT) statically links the following modules into its binary. Each is
listed with its license as of the module versions pinned in `go.mod` /
`go.sum` (verified 2026-08-11). Full license texts follow the table.

| Module | Version | License |
|--------|---------|---------|
| github.com/dustin/go-humanize | v1.0.1 | MIT |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/mattn/go-isatty | v0.0.20 | MIT |
| github.com/ncruces/go-strftime | v1.0.0 | MIT |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| golang.org/x/exp | v0.0.0-20251023183803-a4bb9ffd2546 | BSD-3-Clause |
| golang.org/x/sys | v0.37.0 | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | MIT or Apache-2.0 (dual) |
| modernc.org/libc | v1.67.6 | BSD-3-Clause |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause |
| modernc.org/memory | v1.11.0 | BSD-3-Clause |
| modernc.org/sqlite | v1.46.1 | BSD-3-Clause |

Modules present only in the test dependency graph (not linked into the
binary, e.g. `github.com/google/pprof`, `github.com/hashicorp/golang-lru/v2`)
are not listed here.

---

## BSD 3-Clause License

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.
3. Neither the name of the copyright holder nor the names of its contributors
   may be used to endorse or promote products derived from this software
   without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.

### Copyright holders

- Copyright (c) 2009, 2014 Google Inc. (github.com/google/uuid)
- Copyright (c) 2012 The Go Authors (github.com/remyoudompheng/bigfft)
- Copyright 2009 The Go Authors (golang.org/x/exp, golang.org/x/sys)
- Copyright (c) 2014 The mathutil Authors (modernc.org/mathutil)
- Copyright (c) 2017 The Libc Authors (modernc.org/libc)
- Copyright (c) 2017 The Memory Authors (modernc.org/memory)
- Copyright (c) 2017 The Sqlite Authors (modernc.org/sqlite)

---

## MIT License

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

### Copyright holders

- Copyright (c) 2005-2008 Dustin Sallings (github.com/dustin/go-humanize)
- Copyright (c) Yasuhiro MATSUMOTO (github.com/mattn/go-isatty)
- Copyright (c) 2022 Nuno Cruces (github.com/ncruces/go-strftime)
- Copyright (c) 2006-2010 Kirill Simonov, and contributors (gopkg.in/yaml.v3;
  this module is additionally available under Apache-2.0 — see
  <https://www.apache.org/licenses/LICENSE-2.0>)
