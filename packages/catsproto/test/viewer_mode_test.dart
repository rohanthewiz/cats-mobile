import 'dart:io';

import 'package:test/test.dart';

/// Layer 4 of the four-layer "the phone never resizes the desktop" guard.
///
/// The other three are all inside the type system — CatsConnection builds the
/// Init itself, send() refuses Resize, and the generator's @Deprecated plus
/// analysis_options.yaml make constructing one a build error. This one reads
/// the source, because the failure it guards against is somebody adding a
/// fourth path that the first three do not cover: a raw `socket.send` of a
/// hand-built JSON string, say, or a helper that "just needs" to declare a
/// grid for one screen.
///
/// It is ten lines and it is the layer that would actually catch that.
void main() {
  test('no source under lib/ constructs a Resize or declares a grid', () {
    final offenders = <String>[];
    final dir = Directory('lib');
    for (final file in dir.listSync(recursive: true).whereType<File>()) {
      if (!file.path.endsWith('.dart')) continue;
      // The generated wire layer necessarily DEFINES Resize and its fields —
      // it is a real message the browser sends. What must not exist is a use.
      final generated = file.path.contains('/generated/');
      final lines = file.readAsLinesSync();
      for (var i = 0; i < lines.length; i++) {
        final line = lines[i];
        if (line.trimLeft().startsWith('//') ||
            line.trimLeft().startsWith('///')) {
          continue;
        }
        if (!generated && line.contains('Resize(')) {
          offenders.add('${file.path}:${i + 1}: constructs a Resize');
        }
        final cols = RegExp(r'\bcols:\s*([0-9]+)').firstMatch(line);
        if (!generated && cols != null && cols.group(1) != '0') {
          offenders.add(
            '${file.path}:${i + 1}: declares cols: ${cols.group(1)}',
          );
        }
        final rows = RegExp(r'\brows:\s*([0-9]+)').firstMatch(line);
        if (!generated && rows != null && rows.group(1) != '0') {
          offenders.add(
            '${file.path}:${i + 1}: declares rows: ${rows.group(1)}',
          );
        }
      }
    }
    expect(
      offenders,
      isEmpty,
      reason:
          'A cats client is a viewer. catway takes the session grid from '
          'the first init that declares one and shares it with every '
          'connection, so a phone announcing its own size reflows the '
          "desktop's panes for everybody.",
    );
  });
}
