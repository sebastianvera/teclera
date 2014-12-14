#include <XBee.h>
#include <SoftwareSerial.h>

SoftwareSerial nss = SoftwareSerial(3,2);

XBee xbee = XBee();

Rx16IoSampleResponse ioSample = Rx16IoSampleResponse();
char serialPackageDelimeter = '>';

String inputString = "";         // a string to hold incoming data
boolean stringComplete = false;  // whether the string is complete

void setup() { 
  Serial.begin(9600);
  nss.begin(9600);
  xbee.setSerial(nss);
  turnOffLedsBroadcast();
  inputString.reserve(128);
}

void loop() {
  xbee.readPacket();

  if (xbee.getResponse().isAvailable()) {
    if (xbee.getResponse().getApiId() == RX_16_IO_RESPONSE) {
      xbee.getResponse().getRx16IoSampleResponse(ioSample);
      uint16_t address = ioSample.getRemoteAddress16();

      if (ioSample.containsDigital()) {
         for (int pin = 0; pin <= 3; pin++) {
           if (ioSample.isDigitalEnabled(pin)) {
             printButtonPressedJSON(address, pin);
            } 
          }
      }
    } 
    else {
      // Debug purpose only.
      //Serial.print("Expected I/O Sample, but got ");
      //Serial.print(xbee.getResponse().getApiId(), HEX);
    }
  } 
  else if (xbee.getResponse().isError()) {
    //Debugging purpose
    //Serial.print("Error reading packet.  Error code: ");  
    //Serial.println(xbee.getResponse().getErrorCode());
  }
  
  if (stringComplete) {
    handleCommand(inputString); 
    // clear the string:
    inputString = "";
    stringComplete = false;
  }
}

void turnOffLeds(uint16_t address) {
  for (int buttonPin = 0; buttonPin <= 3; buttonPin++) {
    setValueToLed(address, buttonPin, LOW); 
  }
}


void turnOnLed(uint8_t address, int buttonPin) {
  setValueToLed(address, buttonPin, HIGH);
}

void turnOnFirstTwoLeds() {
  for (int pin = 0; pin < 2; pin++) {
    setValueToLed(BROADCAST_ADDRESS, pin, HIGH);
  }
}

void turnOnLedsBroadcast() {
  broadcastValueToLeds(HIGH);
}

void turnOffLedsBroadcast() {
  for (int pin = 0; pin < 4; pin++) {
    setValueToLed(BROADCAST_ADDRESS, pin, LOW);
  }
}

void setValueToLed(uint16_t address, int buttonPin, int value) {
    uint8_t cmd[] = { 'D', char('4'+ buttonPin) };
    uint8_t val[1];

    if (value == HIGH) {
      val[0] = 0x5;
    } else if (value == LOW) {
      val[0] = 0x4;
    }
    
    RemoteAtCommandRequest remoteAtRequest = RemoteAtCommandRequest(address, cmd, val, sizeof(val));
    xbee.send(remoteAtRequest); 
}

void broadcastValueToLeds(int value) {
   for (int buttonPin = 0; buttonPin <= 3; buttonPin++) {
    setValueToLed(BROADCAST_ADDRESS, buttonPin, value); 
  }
}

void handleCommand(String input) {
  String command = input.substring(0,2);
  
  if(command == "AN") {
    uint16_t decAddress, decPin;
    String address = input.substring(3,5);
    String pin = input.substring(6);
    char hex[2] = { address[0], address[1] };
  
    address.toUpperCase();
    pin.toUpperCase();
    
    sscanf(hex, "%x", &decAddress);
    hex[0] = pin[0]; hex[1] = pin[1];
    sscanf(hex, "%x", &decPin);
    
    turnOffLeds(decAddress);
    turnOnLed(decAddress, decPin);
   } 
   else if (command == "Q1") {
     String mode = input.substring(3, 6);
     mode.toUpperCase();
     if(mode == "TWO") {
       turnOnFirstTwoLeds();
     } else if (mode == "MUL") {
       turnOnLedsBroadcast();
     } else {
       //Debugging purpose
       //Serial.print("Wrong start question type: "); Serial.println(mode);
     }
     
    }
    else if (command == "Q0") {
      turnOffLedsBroadcast();
    }
    else {
      //Debugging purpose
      //Serial.print("Bad command: "); Serial.println(command);
    }
}

void printButtonPressedJSON(uint16_t address, int digitalPin) {
   Serial.print("{ \"address\": ");
   Serial.print(address);
   Serial.print(", \"buttonPressed\": ");
   Serial.print(digitalPin);
   Serial.println(" }");
 }
 
 void serialEvent() {
  while (Serial.available()) {
    char inChar = (char)Serial.read(); 
    if (inChar == serialPackageDelimeter) {
      stringComplete = true;
    } else {
      inputString += inChar;
    }
  }
}
