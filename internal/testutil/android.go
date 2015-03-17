package testutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"v.io/x/devtools/internal/collect"
	"v.io/x/devtools/internal/tool"
	"v.io/x/devtools/internal/util"
)

const (
	defaultAndroidTestTimeout = 20 * time.Minute
	androidManifest           = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
	package="io.v.testing.app"
	android:versionCode="1"
	android:versionName="1.0">
	<uses-permission android:name="android.permission.INTERNET" />
	<uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE"/>
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE"/>
	<application android:label="@string/app_name" android:icon="@drawable/ic_launcher">
		<activity android:name="VeyronBuildActivity"
			android:label="@string/app_name">
			<intent-filter>
				<action android:name="android.intent.action.MAIN" />
				<category android:name="android.intent.category.LAUNCHER" />
			</intent-filter>
		</activity>
	</application>
</manifest>`
	antPropertiesTmplStr = `source.dir={{ range $idx, $src := .Sources }}{{ if gt $idx 0 }};{{ end }}{{ $src }}{{ end }}
`
)

var (
	antPropertiesTmpl = template.Must(template.New("antProperties").Parse(antPropertiesTmplStr))
)

type androidAntProperties struct {
	Sources []string
}

// vanadiumAndroidBuild tests that all Java and Go JNI files build.
func vanadiumAndroidBuild(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"mobile"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)

	// Build an Android app.
	appDir, err := ctx.Run().TempDir("", "app")
	if err != nil {
		return nil, err
	}
	if err := buildAndroidApp(ctx, appDir); err != nil {
		return nil, err
	}
	// Change to the app directory and run "ant debug" to test that the code compiles.
	if err := ctx.Run().Chdir(appDir); err != nil {
		return nil, err
	}
	if err := ctx.Run().TimedCommand(defaultAndroidTestTimeout, "ant", "debug"); err != nil {
		return nil, err
	}
	return &TestResult{Status: TestPassed}, nil
}

// vanadiumAndroidTest runs all Android tests.
func vanadiumAndroidTest(ctx *tool.Context, testName string, _ ...TestOpt) (_ *TestResult, e error) {
	// Initialize the test.
	cleanup, err := initTest(ctx, testName, []string{"mobile"})
	if err != nil {
		return nil, internalTestError{err, "Init"}
	}
	defer collect.Error(func() error { return cleanup() }, &e)
	rootDir, err := util.VanadiumRoot()
	if err != nil {
		return nil, err
	}

	// Build an Android app.
	baseDir, err := ctx.Run().TempDir("", "app")
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(baseDir, "app")
	if err := buildAndroidApp(ctx, appDir); err != nil {
		return nil, err
	}
	// Attach a test project to the app.
	testDir := filepath.Join(baseDir, "test")
	testProjectName := "test"
	toolArgs := []string{"create", "test-project", "--name", testProjectName, "--path", testDir, "--main", appDir}
	if err := ctx.Run().Command(androidTool(rootDir), toolArgs...); err != nil {
		return nil, err
	}
	// Point the test 'src' directory to the veyron test dir.
	javaTestSrcDir := filepath.Join(rootDir, "release", "java", "src", "test", "java")
	javaAppTestSrcDir := filepath.Join(testDir, "src")
	if err := ctx.Run().RemoveAll(javaAppTestSrcDir); err != nil {
		return nil, err
	}
	if err := ctx.Run().Symlink(javaTestSrcDir, javaAppTestSrcDir); err != nil {
		return nil, err
	}
	// Change to the test project directory and run all tests.
	if err := ctx.Run().Chdir(testDir); err != nil {
		return nil, err
	}
	if err := ctx.Run().TimedCommand(defaultAndroidTestTimeout, "ant", "debug", "install", "test"); err != nil {
		return nil, err
	}
	return &TestResult{Status: TestPassed}, nil
}

func buildAndroidApp(ctx *tool.Context, appDir string) error {
	if err := ctx.Run().Command("which", "ant"); err != nil {
		return fmt.Errorf("Couldn't find 'ant' executable.")
	}
	rootDir, err := util.VanadiumRoot()
	if err != nil {
		return err
	}
	// Create an Android project.
	appTargetID := "1"
	appProjectName := "VanadiumBuildApp"
	appActivityName := "VanadiumBuildActivity"
	appPackageName := "io.v.testing.app"
	toolArgs := []string{"create", "project", "--target", appTargetID, "--name", appProjectName, "--path", appDir, "--activity", appActivityName, "--package", appPackageName}
	if err := ctx.Run().Command(androidTool(rootDir), toolArgs...); err != nil {
		return err
	}
	if err := buildAndroidLibs(ctx, appDir); err != nil {
		return err
	}
	// Write a new AndroidManifest file.
	manifestFileName := filepath.Join(appDir, "AndroidManifest.xml")
	if err := ctx.Run().WriteFile(manifestFileName, []byte(androidManifest), 0600); err != nil {
		return err
	}
	// Create a new ant.properties file.
	javaMainSrcDir := filepath.Join(rootDir, "release", "java", "src", "main", "java")
	javaVdlSrcDir := filepath.Join(rootDir, "release", "java", "src", "vdl", "java")
	antProperties := androidAntProperties{
		Sources: []string{javaMainSrcDir, javaVdlSrcDir},
	}
	return createAndroidAntFile(ctx, appDir, antProperties)
}

func buildAndroidLibs(ctx *tool.Context, appDir string) error {
	rootDir, err := util.VanadiumRoot()
	if err != nil {
		return err
	}
	tmpDir, err := ctx.Run().TempDir("", "")
	if err != nil {
		return err
	}
	libsDir := filepath.Join(appDir, "libs")
	if err := ctx.Run().MkdirAll(libsDir, 0700); err != nil {
		return err
	}
	nativeLibsDir := filepath.Join(libsDir, "armeabi-v7a")
	if err := ctx.Run().MkdirAll(nativeLibsDir, 0700); err != nil {
		return err
	}

	// Build the vanadium android library.
	jniLibName := "libveyronjni.so"
	jniLibPath := filepath.Join(tmpDir, jniLibName)
	buildArgs := []string{"xgo", "armv7-android", "build", "-o", jniLibPath, "-ldflags=\"-shared\"", "-tags", "android", "v.io/jni"}
	if err := ctx.Run().Command("v23", buildArgs...); err != nil {
		return err
	}
	// Link vanadium android library into the app native lib dir.
	if err := ctx.Run().Symlink(jniLibPath, filepath.Join(nativeLibsDir, jniLibName)); err != nil {
		return err
	}
	// Link jni wrapper library into the app native lib dir.
	wrapperLibName := "libjniwrapper.so"
	wrapperLibPath := filepath.Join(rootDir, "environment", "cout", "jni-wrapper-1.0", "android", "lib", wrapperLibName)
	if err := ctx.Run().Symlink(wrapperLibPath, filepath.Join(nativeLibsDir, wrapperLibName)); err != nil {
		return err
	}
	// Link third-party Java libraries into the app lib dir.
	thirdPartyDir := filepath.Join(rootDir, "third_party", "java")
	guavaLibName := "guava-17.0.jar"
	guavaLibPath := filepath.Join(thirdPartyDir, "guava-17.0", guavaLibName)
	if err := ctx.Run().Symlink(guavaLibPath, filepath.Join(libsDir, guavaLibName)); err != nil {
		return err
	}
	jodaLibName := "joda-time-2.3.jar"
	jodaLibPath := filepath.Join(thirdPartyDir, "joda-time-2.3", jodaLibName)
	return ctx.Run().Symlink(jodaLibPath, filepath.Join(libsDir, jodaLibName))
}

func androidTool(rootDir string) string {
	sdkLoc := os.Getenv("ANDROID_SDK_HOME")
	if len(sdkLoc) == 0 {
		sdkLoc = filepath.Join(rootDir, "environment", "android", "android-sdk-linux")
	}
	return filepath.Join(sdkLoc, "tools", "android")
}

func createAndroidAntFile(ctx *tool.Context, appDir string, properties androidAntProperties) error {
	var buf bytes.Buffer
	if err := antPropertiesTmpl.Execute(&buf, properties); err != nil {
		return fmt.Errorf("Couldn't generate 'ant.properties' template: %v", err)
	}
	antPropertiesFileName := filepath.Join(appDir, "ant.properties")
	return ctx.Run().WriteFile(antPropertiesFileName, buf.Bytes(), 0600)
}
