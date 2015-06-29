// Copyright 2015 Google Inc. All rights reserved
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kati

import (
	"fmt"
	"io"
	"strings"
	"time"
)

var shBuiltins = []struct {
	name    string
	pattern expr
	compact func(*funcShell, []Value) Value
}{
	{
		name: "android:rot13",
		// in repo/android/build/core/definisions.mk
		// echo $(1) | tr 'a-zA-Z' 'n-za-mN-ZA-M'
		pattern: expr{
			literal("echo "),
			matchVarref{},
			literal(" | tr 'a-zA-Z' 'n-za-mN-ZA-M'"),
		},
		compact: func(sh *funcShell, matches []Value) Value {
			return &funcShellAndroidRot13{
				funcShell: sh,
				v:         matches[0],
			}
		},
	},
	{
		name: "android:find-subdir-assets",
		// in repo/android/build/core/definitions.mk
		// if [ -d $1 ] ; then cd $1 ; find ./ -not -name '.*' -and -type f -and -not -type l ; fi
		pattern: expr{
			literal("if [ -d "),
			matchVarref{},
			literal(" ] ; then cd "),
			matchVarref{},
			literal(" ; find ./ -not -name '.*' -and -type f -and -not -type l ; fi"),
		},
		compact: func(sh *funcShell, v []Value) Value {
			if v[0] != v[1] {
				return sh
			}
			androidFindCache.init(nil)
			return &funcShellAndroidFindFileInDir{
				funcShell: sh,
				dir:       v[0],
			}
		},
	},
	{
		name: "android:all-java-files-under",
		// in repo/android/build/core/definitions.mk
		// cd ${LOCAL_PATH} ; find -L $1 -name "*.java" -and -not -name ".*"
		pattern: expr{
			literal("cd "),
			matchVarref{},
			literal(" ; find -L "),
			matchVarref{},
			literal(` -name "*.java" -and -not -name ".*"`),
		},
		compact: func(sh *funcShell, v []Value) Value {
			androidFindCache.init(nil)
			return &funcShellAndroidFindExtFilesUnder{
				funcShell: sh,
				chdir:     v[0],
				roots:     v[1],
				ext:       ".java",
			}
		},
	},
	{
		name: "android:all-proto-files-under",
		// in repo/android/build/core/definitions.mk
		// cd $(LOCAL_PATH) ; \
		// find -L $(1) -name "*.proto" -and -not -name ".*"
		pattern: expr{
			literal("cd "),
			matchVarref{},
			literal(" ; find -L "),
			matchVarref{},
			literal(" -name \"*.proto\" -and -not -name \".*\""),
		},
		compact: func(sh *funcShell, v []Value) Value {
			androidFindCache.init(nil)
			return &funcShellAndroidFindExtFilesUnder{
				funcShell: sh,
				chdir:     v[0],
				roots:     v[1],
				ext:       ".proto",
			}
		},
	},
	{
		name: "android:java_resource_file_groups",
		// in repo/android/build/core/base_rules.mk
		// cd ${TOP_DIR}${LOCAL_PATH}/${dir} && find . -type d -a \
		// -name ".svn" -prune -o -type f -a \! -name "*.java" \
		// -a \! -name "package.html" -a \! -name "overview.html" \
		// -a \! -name ".*.swp" -a \! -name ".DS_Store" \
		// -a \! -name "*~" -print )
		pattern: expr{
			literal("cd "),
			matchVarref{},
			matchVarref{},
			mustLiteralRE("(/)"),
			matchVarref{},
			literal(` && find . -type d -a -name ".svn" -prune -o -type f -a \! -name "*.java" -a \! -name "package.html" -a \! -name "overview.html" -a \! -name ".*.swp" -a \! -name ".DS_Store" -a \! -name "*~" -print `),
		},
		compact: func(sh *funcShell, v []Value) Value {
			androidFindCache.init(nil)
			return &funcShellAndroidFindJavaResourceFileGroup{
				funcShell: sh,
				dir:       expr(v),
			}
		},
	},
	{
		name: "android:subdir_cleanspecs",
		// in repo/android/build/core/cleanspec.mk
		// build/tools/findleaves.py --prune=$(OUT_DIR) --prune=.repo --prune=.git . CleanSpec.mk)
		pattern: expr{
			literal("build/tools/findleaves.py --prune="),
			matchVarref{},
			literal(" --prune=.repo --prune=.git . CleanSpec.mk"),
		},
		compact: func(sh *funcShell, v []Value) Value {
			if !contains(androidDefaultLeafNames, "CleanSpec.mk") {
				return sh
			}
			androidFindCache.init(nil)
			return &funcShellAndroidFindleaves{
				funcShell: sh,
				prunes: []Value{
					v[0],
					literal(".repo"),
					literal(".git"),
				},
				dirlist:  literal("."),
				name:     literal("CleanSpec.mk"),
				mindepth: -1,
			}
		},
	},
	{
		name: "android:subdir_makefiles",
		// in repo/android/build/core/main.mk
		// build/tools/findleaves.py --prune=$(OUT_DIR) --prune=.repo --prune=.git $(subdirs) Android.mk
		pattern: expr{
			literal("build/tools/findleaves.py --prune="),
			matchVarref{},
			literal(" --prune=.repo --prune=.git "),
			matchVarref{},
			literal(" Android.mk"),
		},
		compact: func(sh *funcShell, v []Value) Value {
			if !contains(androidDefaultLeafNames, "Android.mk") {
				return sh
			}
			androidFindCache.init(nil)
			return &funcShellAndroidFindleaves{
				funcShell: sh,
				prunes: []Value{
					v[0],
					literal(".repo"),
					literal(".git"),
				},
				dirlist:  v[1],
				name:     literal("Android.mk"),
				mindepth: -1,
			}
		},
	},
	{
		name: "android:first-makefiles-under",
		// in repo/android/build/core/definisions.mk
		// build/tools/findleaves.py --prune=$(OUT_DIR) --prune=.repo --prune=.git \
		// --mindepth=2 $(1) Android.mk
		pattern: expr{
			literal("build/tools/findleaves.py --prune="),
			matchVarref{},
			literal(" --prune=.repo --prune=.git --mindepth=2 "),
			matchVarref{},
			literal(" Android.mk"),
		},
		compact: func(sh *funcShell, v []Value) Value {
			if !contains(androidDefaultLeafNames, "Android.mk") {
				return sh
			}
			androidFindCache.init(nil)
			return &funcShellAndroidFindleaves{
				funcShell: sh,
				prunes: []Value{
					v[0],
					literal(".repo"),
					literal(".git"),
				},
				dirlist:  v[1],
				name:     literal("Android.mk"),
				mindepth: 2,
			}
		},
	},
	{
		name: "shell-date",
		pattern: expr{
			mustLiteralRE(`date \+(\S+)`),
		},
		compact: compactShellDate,
	},
	{
		name: "shell-date-quoted",
		pattern: expr{
			mustLiteralRE(`date "\+([^"]+)"`),
		},
		compact: compactShellDate,
	},
}

type funcShellAndroidRot13 struct {
	*funcShell
	v Value
}

func rot13(buf []byte) {
	for i, b := range buf {
		// tr 'a-zA-Z' 'n-za-mN-ZA-M'
		if b >= 'a' && b <= 'z' {
			b += 'n' - 'a'
			if b > 'z' {
				b -= 'z' - 'a' + 1
			}
		} else if b >= 'A' && b <= 'Z' {
			b += 'N' - 'A'
			if b > 'Z' {
				b -= 'Z' - 'A' + 1
			}
		}
		buf[i] = b
	}
}

func (f *funcShellAndroidRot13) Eval(w io.Writer, ev *Evaluator) error {
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.v)
	if err != nil {
		return err
	}
	rot13(fargs[0])
	w.Write(fargs[0])
	freeBuf(abuf)
	return nil
}

type funcShellAndroidFindFileInDir struct {
	*funcShell
	dir Value
}

func (f *funcShellAndroidFindFileInDir) Eval(w io.Writer, ev *Evaluator) error {
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.dir)
	if err != nil {
		return err
	}
	dir := string(trimSpaceBytes(fargs[0]))
	freeBuf(abuf)
	logf("shellAndroidFindFileInDir %s => %s", f.dir.String(), dir)
	if strings.Contains(dir, "..") {
		logf("shellAndroidFindFileInDir contains ..: call original shell")
		return f.funcShell.Eval(w, ev)
	}
	if !androidFindCache.ready() {
		logf("shellAndroidFindFileInDir androidFindCache is not ready: call original shell")
		return f.funcShell.Eval(w, ev)
	}
	sw := ssvWriter{w: w}
	androidFindCache.findInDir(&sw, dir)
	return nil
}

type funcShellAndroidFindExtFilesUnder struct {
	*funcShell
	chdir Value
	roots Value
	ext   string
}

func (f *funcShellAndroidFindExtFilesUnder) Eval(w io.Writer, ev *Evaluator) error {
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.chdir, f.roots)
	if err != nil {
		return err
	}
	chdir := string(trimSpaceBytes(fargs[0]))
	var roots []string
	hasDotDot := false
	ws := newWordScanner(fargs[1])
	for ws.Scan() {
		root := string(ws.Bytes())
		if strings.Contains(root, "..") {
			hasDotDot = true
		}
		roots = append(roots, string(ws.Bytes()))
	}
	freeBuf(abuf)
	logf("shellAndroidFindExtFilesUnder %s,%s => %s,%s", f.chdir.String(), f.roots.String(), chdir, roots)
	if strings.Contains(chdir, "..") || hasDotDot {
		logf("shellAndroidFindExtFilesUnder contains ..: call original shell")
		return f.funcShell.Eval(w, ev)
	}
	if !androidFindCache.ready() {
		logf("shellAndroidFindExtFilesUnder androidFindCache is not ready: call original shell")
		return f.funcShell.Eval(w, ev)
	}
	buf := newBuf()
	sw := ssvWriter{w: buf}
	for _, root := range roots {
		if !androidFindCache.findExtFilesUnder(&sw, chdir, root, f.ext) {
			freeBuf(buf)
			logf("shellAndroidFindExtFilesUnder androidFindCache couldn't handle: call original shell")
			return f.funcShell.Eval(w, ev)
		}
	}
	w.Write(buf.Bytes())
	freeBuf(buf)
	return nil
}

type funcShellAndroidFindJavaResourceFileGroup struct {
	*funcShell
	dir Value
}

func (f *funcShellAndroidFindJavaResourceFileGroup) Eval(w io.Writer, ev *Evaluator) error {
	abuf := newBuf()
	fargs, err := ev.args(abuf, f.dir)
	if err != nil {
		return err
	}
	dir := string(trimSpaceBytes(fargs[0]))
	freeBuf(abuf)
	logf("shellAndroidFindJavaResourceFileGroup %s => %s", f.dir.String(), dir)
	if strings.Contains(dir, "..") {
		logf("shellAndroidFindJavaResourceFileGroup contains ..: call original shell")
		return f.funcShell.Eval(w, ev)
	}
	if !androidFindCache.ready() {
		logf("shellAndroidFindJavaResourceFileGroup androidFindCache is not ready: call original shell")
		return f.funcShell.Eval(w, ev)
	}
	sw := ssvWriter{w: w}
	androidFindCache.findJavaResourceFileGroup(&sw, dir)
	return nil
}

type funcShellAndroidFindleaves struct {
	*funcShell
	prunes   []Value
	dirlist  Value
	name     Value
	mindepth int
}

func (f *funcShellAndroidFindleaves) Eval(w io.Writer, ev *Evaluator) error {
	if !androidFindCache.leavesReady() {
		logf("shellAndroidFindleaves androidFindCache is not ready: call original shell")
		return f.funcShell.Eval(w, ev)
	}
	abuf := newBuf()
	var params []Value
	params = append(params, f.name)
	params = append(params, f.dirlist)
	params = append(params, f.prunes...)
	fargs, err := ev.args(abuf, params...)
	if err != nil {
		return err
	}
	name := string(trimSpaceBytes(fargs[0]))
	var dirs []string
	ws := newWordScanner(fargs[1])
	for ws.Scan() {
		dir := string(ws.Bytes())
		if strings.Contains(dir, "..") {
			logf("shellAndroidFindleaves contains .. in %s: call original shell", dir)
			return f.funcShell.Eval(w, ev)
		}
		dirs = append(dirs, dir)
	}
	var prunes []string
	for _, arg := range fargs[2:] {
		prunes = append(prunes, string(trimSpaceBytes(arg)))
	}
	freeBuf(abuf)

	sw := ssvWriter{w: w}
	for _, dir := range dirs {
		androidFindCache.findleaves(&sw, dir, name, prunes, f.mindepth)
	}
	return nil
}

var (
	// ShellDateTimestamp is an timestamp used for $(shell date).
	ShellDateTimestamp time.Time
	shellDateFormatRef = map[string]string{
		"%Y": "2006",
		"%m": "01",
		"%d": "02",
		"%H": "15",
		"%M": "04",
		"%S": "05",
		"%b": "Jan",
		"%k": "15", // XXX
	}
)

type funcShellDate struct {
	*funcShell
	format string
}

func compactShellDate(sh *funcShell, v []Value) Value {
	if ShellDateTimestamp.IsZero() {
		return sh
	}
	tf, ok := v[0].(literal)
	if !ok {
		return sh
	}
	tfstr := string(tf)
	for k, v := range shellDateFormatRef {
		tfstr = strings.Replace(tfstr, k, v, -1)
	}
	return &funcShellDate{
		funcShell: sh,
		format:    tfstr,
	}
}

func (f *funcShellDate) Eval(w io.Writer, ev *Evaluator) error {
	fmt.Fprint(w, ShellDateTimestamp.Format(f.format))
	return nil
}
