# Using AFK

## Dictation and the floating bubble

Choose a Whisper model and language in Studio, then toggle dictation with the
record button or your global shortcut. The default shortcut is `Ctrl+Shift+Space`;
configure or disable it in Settings. Built-in global shortcuts currently use KDE
KGlobalAccel. Other desktops can integrate `afk-desktop --toggle` with their own
shortcut settings.

Click the bubble's animated indicator to expand or collapse live text. The
indicator is an animation, not an audio-level meter. Settings offers nine screen
anchors, applied within the primary monitor's work area. Reliable positioning
requires X11/XWayland.

Pause briefly after your final word before stopping: live text is provisional
and the last audio buffer is not guaranteed to be flushed. Stopping dictation
copies text to the clipboard. Models are released after a session and loaded
again when the next starts.

Closing a window hides it and leaves AFK running in the tray. It does not stop
an active recording. The tray offers Open, Transcribe file, Choose Model and
Close. Choose Model lists installed models while idle; Close exits AFK.

## Meeting recordings

1. Open **Meetings** and choose **Your microphone** and **Meeting audio**.
2. Select **Record meeting**, then **Stop & save** when finished.
3. Prepare Whisper and Nemotron in Model Garage if needed.
4. Open a recording from **Meeting history**, then choose **Process recording**.
5. Read its transcript on that meeting’s page. Edit a speaker’s name and select
   **Save name**; copied transcripts use your saved names. A blank name restores
   the original label.

Recording does not require a model and does not run transcription. Processing
uses the current Whisper model and language. It labels the microphone as **You**
and assigns generic labels to remote speakers, with up to eight remote speakers.
Labels are local to the recording, not enrolled voice identities.

Both inputs support **System default**, resolved when recording starts. A running
recording keeps the selected devices even if system defaults change. Meeting
audio can be an output monitor or an active application stream. Start playback
in your meeting app before refreshing the list. If the app recreates its stream,
refresh and select it again. A disconnected source stops capture and retains
what has already been recorded.

Use headphones: AFK does not implement acoustic echo cancellation. Speaker audio
picked up by your microphone can appear twice in the transcript.

Saved recordings survive restarts. Canceling processing preserves the audio for
another attempt. Reprocessing replaces that meeting’s transcript; canceling
preserves the previously completed transcript. Speaker names survive restarts
and reprocessing, but should be checked after reprocessing because labels can
be reassigned. Meetings do not populate Studio or dictation history.

### Storage and limits

The data directory contains `meetings/<timestamp>/` folders with
`microphone.wav`, `meeting-audio.wav` and `recording.json`. Processing adds logs
and transcript files. Custom speaker names are stored in `speakers.json`.
Audio and meeting transcripts remain saved even with dictation history disabled; deleting
history does not remove these folders.

Two mono PCM16 tracks consume about **230 MB per hour**. Free disk and the WAV
container limit apply, approximately 37 hours per track. There is no recording
duration timer, but multi-hour endurance has not been verified.

Processing uses 28-second Whisper chunks with one second of previous context.
Nemotron maintains speaker state between chunks. Both models may occupy GPU
memory during processing; neither runs during capture. Timings, overlapping
speech and speaker assignments remain estimates.

## Audio files

Choose **Transcribe file** from the tray or use Studio's file import panel.
WAV, MP3, M4A, FLAC, Ogg and other FFmpeg-readable formats are supported. AFK
converts the first audio track to 16 kHz mono PCM; the original file is unchanged.
Enable **Identify speakers** after preparing Nemotron to add timestamped turns.
Overlapping speech can be marked “Overlapping speakers”; words outside detected
speech can be marked “Unassigned”.

One audio operation runs at a time. Imports can be canceled and keep the bubble
hidden. Completed text appears in Studio and optional history. Imports and
processed meetings do not automatically overwrite the clipboard.

Imports currently decode and transcribe the whole file. There is no application
file-size or duration cap, but RAM, disk space and WAV limits apply. Converted
audio uses about 115 MB of temporary disk space per hour; float audio alone uses
about 230 MB of RAM per hour, in addition to model buffers. Temporary files are
removed on completion, error or cancellation; a crash can leave them behind.
Chunked/resumable file imports are not implemented. File-import history duration
currently reflects processing time rather than source audio duration.

## Local data

Installed builds use `$XDG_DATA_HOME/afk`, normally `~/.local/share/afk`.
Development builds use `.afk/`; `--data /absolute/path` overrides the location.
Settings and optional history are stored in `desktop-state.json`. History keeps
up to 200 transcripts. Dictation audio is not saved to a recording file.

Model downloads require internet access and are verified against pinned checksums.
Prepared models run offline. The packaged app includes its native engines;
development builds compile them during preparation. The first GPU run of a
packaged engine can take longer while the NVIDIA driver compiles CUDA kernels.

The bubble waveform follows microphone volume, with a short scrolling history.
Silence and stopped capture settle to a flat line; loading and processing do not
simulate speech. Dictation meters the default PulseAudio/PipeWire microphone;
meetings meter their selected microphone.
