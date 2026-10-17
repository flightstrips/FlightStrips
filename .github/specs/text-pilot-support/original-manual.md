# Text pilot support — source manual

Source: https://docs.google.com/document/d/1D3AhxGAZiX9-bYWerPIPgErx8YwRUVmy/edit

Retrieved: 2026-10-10. Plain-text export preserved below; formatting from the source export is retained.

Introduction
This manual concerns itself with explanations of text messages to text pilots when instructions are given.
When a text pilot is recognized, the following logic enters, and changes the scope to include the 4 sections described below:
* Strip Changes
* Behavioral changes
* Dialogues
* Individual Dialogue Structure.
When a text is send to a text pilot it is broadcast on the frequency, and the callsign of the pilot in question selected before hand by the system, to ensure it goes to the right pilot.
There is no description of receiving texts from pilot as this is not feasible. The manual below assumes controller is reading in Euroscope, and reacting on background of that.
Strip Changes.
All strips are changed to allow to show the controller that a pilot is text pilot by introduction by a “text pilot field”.
“text pilot field” is defined as a small box in the bottom right corner of:
* “Callsign box” for UNCLEARED, CLEARED and TWR TWY DEP strips.
* “Aircraft type/reg box” of Push/Start strips and APN TWY DEP strips.
* “STAR/ARR” Airport field of the TWR TWY ARR strips.
* Stand field of all APN TWY ARR strips.


the “text pilot field” is of following dimensions for all cases:
* Height: 1/3 of the height of the box it populates (callsign box e.g.)
* Length: 100% of the height of the “text pilot field”.
* Stroke: same stroke as the boxes its located in, but only visible on the left side. All other sides are invisible.
* Fill color: Depending on flight.
   * Full voice: Fill is blank, no Text
   * Receive voice: Fill is blank, text shows “r”
   * Full text: fill is red (FF6D4D). text shows ”t”.
Storyboard: https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3584-371&m=dev
Behavioral Changes.
Follow logic and behavior is changed when an aircraft is identified as “text only”. (receive voice has no impact on behavior).
SI Field
Clicking on the SI Field, opens the “SI TEXT” Dialogue, no matter the ownership.
If under ownership (white):
* if a frequency is selected in the SI TEXT dialogue. (7A766E colored ones), a manual transfer to that frequency will be initiated.
* If a next unit was already identified before clicking that unit will be highlighted (70ED45) when opening the “SI TEXT” Dialogue
If transfer is already initiated (half white half Orange).
* Selection of “Remain this frequency” by itself or in any combination with other words, in the dialogue will reassume the strip. All other selections will not affect the transfer, but simply send the text message to the pilot.
If transfer is receiving (half purple half white).
* Clicking on the SI field will NOT open the dialogue.
Any other combinations (full Purple, full orange) will open the dialogue and result in only the sending of text message to the pilot on the active frequency.
PUSHBACK, all Taxi maps
Clicking on a Hold Short position, a Release point, a Direction or Holding Point (any button), will first open the appropriate dialogue, then both dialogues (Taxi map and text pilot dialuge) closes, and a text message is sent.
TKOFF/LDG CLX button
Clicking on the takeoff or land button when a text pilot is sensed, will open a “TKOFF Dialogue” to be able to assign the appropriate instructions.
Stand assignment
Clicking on a stand will normally open the stand assignment. Clicking SEND REQ will first open the taxi-in dialogue of tower or apron, depending on which entity clicks it.


Dialogues
6 unique dialogues are available for providing instructions to text pilots.
* PUSH/START: used when a release point is selected on a text pilots pushback instruction.
* APRON TAXIOUT: used when on AA+AD / APRON DEP /APRON ARR, and a HS is selected.
* APRON TAXIIN: used when on AA+AD / APRON DEP /APRON ARR, and a HS is selected.
* TOWER TAXIOUT: used when on GW+GE / TW+TE / TWR+GND and a HS or HP is selected.
* TOWER TAXIIN: used when on GW+GE / TW+TE / TWR+GND and a HS or HP is selected.
* SI TEXT: opens whenever SI field is pressed, no matter ownership (except half purple half white).
* TOFF/LDG: Used when the takeoff and/or landing field is pushed.
Either via these or via regular dialogue is text messages send to pilots. Dialogues are defined after the logic is understood.
Dialogue Structure
Opening dialogue consists of:
[CALLSIGN] written in the top, and should be in the format [callsign of flight] “text message”
[Text Field], depicted in white background with black writing. This is where the words selected in the dialogue appears. It can also be deleted by pressing ERASE and/or written as free text.
[Action Fields], depicted in background fill: (D6D6D6) or (7A766E) or (918973), is clicked to add the sentence displayed to the [text field]. The color codes triggers slightly different logic.
* D6D6D6: used for Taxiways and Runways. Should appear with the text in the format: “TWY/RWY [text], “ (RWYs are 22L, 22R, 12, 30, 04R, 04L), TWY are all other combinations.
* 918973: will write the exact text depicted, except not written in capital letters, but only first letter capital.
* 7A766E: Will be written as “Holding point [Value], Runway [RWY selected in strip].”
Once an action field is pushed, it will be highlighted in green background (70ED45), with black font (000000). The wording in that action field (Or an equivalent value defined below), will be written in the text field. If the action field is pressed again, the word (or equivalent value) is removed from text field, and button returns to its default colors.
Clicking [ERASE] lighted up in red (FF6D4D) with white writing (FFFFFF), which will delete any text depicted.
Clicking on the [Text Field] will allow for free text editing.
Clicking [SEND] will send the message to the pilot.
Clicking [ESC] will close the dialogue without sending anything. Everything is reset.
SI TEXT Dialogue
https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3586-471&m=dev
This is a unique one as it is usable for all controller at all times. As defined in the BEHAVIOR CHANGE chapter opening this dialogue will either initiate a transfer or not.
The dialogue consist of a:
* White (FFFFFF) text field that can be used to write custom messages in black (000000).
* Brown (918973) boxes for predetermined messages.
* Gray (7A766E) boxes for handovers. There consist of:
   * [Station name].
   * [Frequency], which must also be used to do manual transfers recognition.
* Dark Gray (9E989C) which cannot be selected.
When opening the dialogue following rules apply:
General Rules:
* Clicking on any section will highlight it in green (70ED45) with black font (000000). Clicking on it again will revert to default.
* Clicking on a section will write on the text field in following manner:
   * 918973 color fill button: Write that exact phrase, except only first letter in capital.
   * 7A766E color fill button: Write: “Contact [Station name] on frequency [Frequency].”
* Clicking on a 7A766E colored fields, will also highlight the dark gray (9E989C ) button.
* If clicking on SI field as owner of the strip WITH a next frequency identified:
   * That frequency if existing in the SI Text dialogue is automatically selected.
* Clicking SEND will send the message and transfer if appropriate.
* Clicking ESC will cancel all actions, including transfer.
Specific Rules:
In addition to the general rules, specific behaviors are in effect when certain buttons are pushed in certain conditions:
CLR DEL scope:
* If SI Field clicked from CLR DEL scope in CLEARED bay:
      * READBACK correct option default selected (70ED45), and
   * Only If 121.905 is primed or cross coupled:
      * CONTACT and KASTRUP APRON 121.905 is also default selected (70ED45).
SEQ PLN
* First time SI field is clicked:
      * READBACK correct option default selected (70ED45), with nothing else selected, regardless of ownership of strip and transfer logic.
* Second time SI field is clicked:
      * No default selection, except as per transfer logic.
ATC CLEARANCE
Does not require a specific dialogue, check that:
If(text pilot = true). Message send to pilot in following format:
“Cleared to [ICAO destination], via ”
If(SID=true), “[Assigned SID] Departure, ”
If(SID=false and Route=notblank), “Flight Plan Route, ”
If(SID=false and Route=blank), “Radar Vectors, ”
“Initial Climb [CFL], ”
“Squawk. ” [Assigned Squawk]
If(SID=false), “After Departure, ”
If(AHDG=NOT BLANK and NOT SA), “Fly Heading [AHDG]”
If(AHDG=SA), “Climb Straight ahead”


When pilot reads back, clicking SI Field will open the SI TEXT dialogue, and in accordance with described in that section, will allow controller to send a readback correct.


PUSH AND START
https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3551-6409&m=dev
After opening the airport map, clicking on a release point will open the PUSH/START TEXT MENU
Here a Default text is present once opened, which shows:
If(Release point selected) “Push and Start Approved, Release point [Point selected].
If(Direction is selected) “Push and Start Approved, Face [Direction].
Taxi out APRON
https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3551-6410&m=dev
Appropriate dialogue opens when any HS instruction is selected via the taxi maps.
After Opening the menu:
“TAXI VIA” is automatic selected, and will be placed in the beginning of the sentence, unless actively deselected.
The value that is selected from taxi map, is selected, and placed in the text field as the end sentence. What is then pressed goes between the two texts. Clicking on a HS position is normally phrased as [Firstletter/number] / [Last Letter/number]
It shall be phrased as
* “TWY” [first letter], “Hold short of” [later letter]. (the slash is ignored).
If the markings A, F, D, B, K3, K2, K1 is selected.
* “TWY [Letter], “Hold Short RWY 12/30”.
* In this case “A” and “Hold Short RWY 12/30” is automatically selected.
Taxi In APRON
https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3551-6819&m=dev
Appropriate dialogue opens when any HS instruction via the HS menu is selected via the taxi maps or clicking on the SEND REQ button in the stand assignment menu.
After Opening via the taxi maps:
“TAXI VIA” is automatic selected, and will be placed in the beginning of the sentence, unless actively deselected.
The value that is selected from taxi map, is selected, and placed in the text field as the end sentence. What is then pressed goes between the two texts. Clicking on a HS position is normally phrased as [Firstletter/number] / [Last Letter/number]
It shall be phrased as
* “TWY” [first letter], “Hold short of” [later letter]. (the slash is ignored).
After Opening via the Stand assignment menu:
“TAXI VIA” is automatic selected, and will be placed in the beginning of the sentence, unless actively deselected.
“to stand: [STAND ASSIGNED]”, is added as the end sentence. It automatically selects the action field [Assigned Stand]. This shall not say assigned stand, but depict the stand which is recognized by the strip.
Taxi out TOWER
https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3551-6411&m=dev
Appropriate dialogue opens when any HS or HP instruction is selected via the taxi maps.
After Opening the menu:
“TAXI VIA” is automatic highlighted, and will be placed in the beginning of the sentence, unless actively deselected.
The value that is selected from the map, populates the [Selected Text] field as well as the [text field] and will be depicted as either:
* “TWY” [first letter], “Hold short of” [later letter]. (the slash is ignored).
If A1, A2, A3, A4, A5, B1, B2, B3, B4, E1, V1, V2, G1, K3, K2, K1 is selected from the HP menu
* “for Holding Point [Letter], RWY [Assigned Runway from strip]”.
In this case the button the letter selected will light up in green background (70ED45), with black font (000000).
Taxi In TOWER
https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3551-6540&m=dev
Appropriate dialogue opens when any HS instruction via the HS menu is selected via the taxi maps or clicking on the SEND REQ button in the stand assignment menu.
“TAXI VIA” is automatic highlighted, and will be placed in the beginning of the sentence, unless actively deselected, no matter how it is opened.
After Opening via the taxi maps from GND+TWR scope:
Same logic as with other dialouges opened via taxi maps.
After Opening via the HS menu from TW+TE or GW+GE scope:
A default selection is directly made based on the default HS value from TW+TE and/or GW+GE which is normally defined in the HS area on these scopes.
K3: “RWY30” “K3” and “HOLD SHORT TWY Z ON K3” items are written and highlighted.
K2: “RWY30” “K2” and “HOLD SHORT TWY K” items are written and highlighted.
B: “Taxi via”, “B”, “CROSS RWY 12/30”, “HOLD SHORT OF TWY Z” items are written and highlighted.
A: “Taxi via”, “A”, “CROSS RWY 12/30”, “HOLD SHORT OF TWY Z” items are written and highlighted.
F: “Taxi via”, “A”, “CROSS RWY 12/30”, “VIA F”, “HOLD SHORT OF TWY Z” items are written and highlighted.
After Opening via the Stand assignment menu:
“TAXI VIA” is automatic selected, and will be placed in the beginning of the sentence, unless actively deselected.
“to stand: [STAND ASSIGNED]”, is added as the end sentence. It automatically selects the action field [Assigned Stand]. This shall not say assigned stand, but depict the stand which is recognized by the strip.
TKOFF/LDG Dialogue
https://www.figma.com/design/csKJuv9WCgO36UjsE1lC2h/Project-FS---Flightstrps-by-VATSCA?node-id=3576-8658&m=dev
When the RWY button is pressed for a text pilot, this dialogue will appear as it is pushed. Depending on the result of the click different messages are displayed. The result of the click will happen BEFORE or SIMULTANIOUSLY as the dialogue opens up, and its result is not affected by what will be selected in the TKOFF/LDG Dialogue.
TAKEOFF PHASE
When the RWY button is clicked for a departure, the dialogue will open.
Following sentences are preselected highlighted in green and added to the text field:
* If result is RED
   * “LINE UP AND WAIT”
   * “RWY [RWY]” (RWY that is written in the strip is selected).
* If result is GREEN
   * [WIND] (Wind taken from Flightstrips recorded wind)
   * “CLEARED FOR TAKEOFF”
   * “RWY [RWY]” (RWY that is written in the strip is selected).
LANDING PHASE
When the RWY button is clicked for an arrival, the dialogue will open.
Following sentences are preselected highlighted in green and added to the text field:
* If result is RED
   * CONTINUE APPROACH
   * “RWY [RWY]” (RWY that is written in the strip is selected).
* If result is GREEN
   * [WIND] (Wind taken from Flightstrips recorded wind)
   * CLEARED TO LAND
   * “RWY [RWY]” (RWY that is written in the strip is selected).
Appendix - Storyboards
STORYBOARD 1 – SI FIELD TRANSFER
STORYBOARD 2 – INBOUND TAXI TO K3
STORYBOARD 3 – ASSIGNING A STAND